package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
)

// TestMerge_DepSatisfiedIssuesMergeInOrder asserts two in-review issues whose
// blocked_by are all done merge in dependency order and both flip to done. The
// dependent's branch carries a commit that only applies cleanly atop the
// blocker's, so an out-of-order merge would conflict — order is load-bearing.
func TestMerge_DepSatisfiedIssuesMergeInOrder(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	branch1 := "tester/demo--1-first"
	makeMergeableBranch(t, fix.root, integrationBranch, branch1, "shared.txt")

	first := mkAFKIssue(1, "first", "in-review")
	first.Branch = &branch1
	first.Blocking = []int{2}
	writeIssue(t, fix.plansDir, first)

	branch2 := "tester/demo--2-second"
	makeMergeableBranch(t, fix.root, integrationBranch, branch2, "second.txt")

	second := mkAFKIssue(2, "second", "in-review", 1)
	second.Branch = &branch2
	writeIssue(t, fix.plansDir, second)

	res, err := d.Merge(context.Background(), "demo")
	require.NoError(t, err)

	assert.ElementsMatch(t, []int{1, 2}, res.Merged)
	assert.Empty(t, res.Conflicted)

	assert.Equal(t, "done", loadIssueJSON(t, fix.plansDir, 1, "first").Status)
	assert.Equal(t, "done", loadIssueJSON(t, fix.plansDir, 2, "second").Status)
}

// TestMerge_SkipsIssuesWithUnsatisfiedDeps asserts an in-review issue whose
// blocked_by is not yet done is excluded from the merge set — the acceptance
// criterion is "every in-review issue whose blocked_by are all done". The
// blocker here is still in-review (not done), so the dependent must not merge.
func TestMerge_SkipsIssuesWithUnsatisfiedDeps(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	// Blocker is in-review but NOT done, and has NO branch — so pendingMergeBack
	// skips it (no branch) and it never reaches done. The dependent's deps are
	// therefore unsatisfied and it must be excluded.
	blocker := mkAFKIssue(1, "blocker", "in-review")
	blocker.Branch = nil
	blocker.Blocking = []int{2}
	writeIssue(t, fix.plansDir, blocker)

	branch2 := "tester/demo--2-dependent"
	makeMergeableBranch(t, fix.root, integrationBranch, branch2, "dep.txt")
	dependent := mkAFKIssue(2, "dependent", "in-review", 1)
	dependent.Branch = &branch2
	writeIssue(t, fix.plansDir, dependent)

	res, err := d.Merge(context.Background(), "demo")
	require.NoError(t, err)

	assert.Empty(t, res.Merged, "dependent with unsatisfied deps must not merge")
	assert.Empty(t, res.Conflicted)
	assert.Equal(t, "in-review", loadIssueJSON(t, fix.plansDir, 2, "dependent").Status,
		"dependent must stay in-review when its blocker is not done")
}

// TestMerge_ConflictMarksBlockedSiblingsStillMerge asserts a branch that
// conflicts with integration is reported in Conflicted, marked blocked, and
// `git merge --abort` leaves the iwt clean so a sibling still merges to done.
func TestMerge_ConflictMarksBlockedSiblingsStillMerge(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	// Seed a commit on integration that edits conflict.txt, then branch off an
	// EARLIER point so the issue branch edits the same file differently —
	// guaranteeing a merge conflict.
	makeConflictingBranch(t, fix.root, integrationBranch, "tester/demo--1-clash", "conflict.txt")
	clash := mkAFKIssue(1, "clash", "in-review")
	branch1 := "tester/demo--1-clash"
	clash.Branch = &branch1
	writeIssue(t, fix.plansDir, clash)

	// A second, cleanly-mergeable sibling.
	branch2 := "tester/demo--2-clean"
	makeMergeableBranch(t, fix.root, integrationBranch, branch2, "clean.txt")
	clean := mkAFKIssue(2, "clean", "in-review")
	clean.Branch = &branch2
	writeIssue(t, fix.plansDir, clean)

	res, err := d.Merge(context.Background(), "demo")
	require.NoError(t, err)

	assert.Equal(t, []int{1}, res.Conflicted, "conflicting issue must be reported")
	assert.Equal(t, []int{2}, res.Merged, "sibling must still merge despite the conflict")

	assert.Equal(t, "blocked", loadIssueJSON(t, fix.plansDir, 1, "clash").Status)
	assert.Equal(t, "done", loadIssueJSON(t, fix.plansDir, 2, "clean").Status)
}

// TestMerge_IdempotentNoOpWhenNothingInReview asserts Merge is a no-op when no
// issue is in-review with a branch: empty result, no error, parent untouched.
func TestMerge_IdempotentNoOpWhenNothingInReview(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "beta", "done"))

	d := newDispatcher(fix)
	res, err := d.Merge(context.Background(), "demo")
	require.NoError(t, err)
	assert.Nil(t, res.Merged)
	assert.Nil(t, res.Conflicted)
}

// TestMerge_DoesNotMoveParentHEAD asserts the parent repo's symbolic ref and
// resolved HEAD are byte-identical before and after a successful Merge — all
// merges happen inside the integration worktree (ADR-0003).
func TestMerge_DoesNotMoveParentHEAD(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	branch := "tester/demo--1-alpha"
	makeMergeableBranch(t, fix.root, integrationBranch, branch, "alpha.txt")
	iss := mkAFKIssue(1, "alpha", "in-review")
	iss.Branch = &branch
	writeIssue(t, fix.plansDir, iss)

	refBefore, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaBefore, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)

	res, err := d.Merge(context.Background(), "demo")
	require.NoError(t, err)
	require.Equal(t, []int{1}, res.Merged)

	refAfter, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaAfter, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(refBefore)), strings.TrimSpace(string(refAfter)),
		"parent symbolic ref must not move")
	assert.Equal(t, strings.TrimSpace(string(shaBefore)), strings.TrimSpace(string(shaAfter)),
		"parent HEAD commit must not move")
}

// TestMerge_RejectsConcurrentMergeOnSameSlug asserts a Merge against a slug
// whose .dispatch.lock is already held fails fast with an error naming the slug.
func TestMerge_RejectsConcurrentMergeOnSameSlug(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))

	lockPath := filepath.Join(fix.plansDir, "demo", ".dispatch.lock")
	release, err := planrepo.TryFlock(lockPath)
	require.NoError(t, err)
	defer release()

	d := newDispatcher(fix)
	start := time.Now()
	_, err = d.Merge(context.Background(), "demo")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond, "must fail fast without polling")
	assert.Contains(t, err.Error(), "demo", "error should name the contended slug")
}

// makeConflictingBranch seeds a commit on integration that writes `file`, then
// creates `branch` off the PRIOR commit and writes a different body to the same
// file — so a later `git merge branch` into integration conflicts.
func makeConflictingBranch(t *testing.T, root, integrationBranch, branch, file string) {
	t.Helper()
	run := func(args ...string) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	run("checkout", integrationBranch)
	base, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	baseSHA := strings.TrimSpace(string(base))

	// Commit on integration touching the file.
	writeAndCommit(t, root, file, "integration side\n", "integration edit")

	// Branch off the earlier base, edit the same file differently.
	run("branch", branch, baseSHA)
	run("checkout", branch)
	writeAndCommit(t, root, file, "branch side\n", "branch edit")
	run("checkout", "main")
}

func writeAndCommit(t *testing.T, root, file, body, msg string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte(body), 0o644))
	for _, args := range [][]string{
		{"add", file},
		{"commit", "-m", msg},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
}
