package worktree

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/require"
)

func initRepo(t *testing.T) string {
	t.Helper()

	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	return root
}

func TestCreate_DeterministicBranchAndPath(t *testing.T) {
	root := initRepo(t)

	res, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup-middleware", "")
	require.NoError(t, err)

	require.Equal(t, "tester/auth--1-setup-middleware", res.Branch)
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	require.NoError(t, err)
	expectedPath := filepath.Join(parent, "repo-wt", "auth", "1-setup-middleware")
	require.Equal(t, expectedPath, res.Path)

	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), res.Path)
	require.Contains(t, string(out), "branch refs/heads/"+res.Branch)
}

// TestCreate_HonorsWorktreeBase asserts that a configured WorktreeBase
// relocates the {repoName}-wt root while preserving the {slug}/{leaf} layout.
func TestCreate_HonorsWorktreeBase(t *testing.T) {
	root := initRepo(t)

	customBase := t.TempDir()
	cfg := config.Config{WorktreeBase: customBase}

	res, err := Create(context.Background(), cfg, root, "auth", 1, "setup", "")
	require.NoError(t, err)

	expectedPath := filepath.Join(customBase, "repo-wt", "auth", "1-setup")
	require.Equal(t, expectedPath, res.Path)

	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), res.Path)
	require.Contains(t, string(out), "branch refs/heads/"+res.Branch)
}

// TestCreate_BranchesOffSuppliedBaseRef asserts that the new branch points at
// the caller-supplied baseRef rather than whatever HEAD happens to be. The
// dispatcher relies on this to root issue branches off the integration branch
// even when the user invoked `pba dispatch` from an unrelated branch.
func TestCreate_BranchesOffSuppliedBaseRef(t *testing.T) {
	root := initRepo(t)

	// Create an integration branch with one extra commit on top of main.
	for _, args := range [][]string{
		{"-C", root, "checkout", "-b", "integration"},
		{"-C", root, "commit", "--allow-empty", "-m", "integration commit"},
		{"-C", root, "checkout", "main"},
	} {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	res, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "integration")
	require.NoError(t, err)

	// New branch's tip must equal integration's tip, NOT main's.
	branchTip, err := exec.Command("git", "-C", root, "rev-parse", res.Branch).Output()
	require.NoError(t, err)
	integrationTip, err := exec.Command("git", "-C", root, "rev-parse", "integration").Output()
	require.NoError(t, err)
	mainTip, err := exec.Command("git", "-C", root, "rev-parse", "main").Output()
	require.NoError(t, err)

	require.Equal(t, strings.TrimSpace(string(integrationTip)), strings.TrimSpace(string(branchTip)))
	require.NotEqual(t, strings.TrimSpace(string(mainTip)), strings.TrimSpace(string(branchTip)))
}

// TestCreate_IdempotentOnRepeatCall asserts that a second Create with the same
// (slug, id, issueSlug) returns the existing branch+path instead of failing
// with "branch already exists". This is the dispatch-loop crash-recovery
// contract — runOne can re-enter without leaving the issue stuck because the
// branch from a prior partial run is still on disk.
func TestCreate_IdempotentOnRepeatCall(t *testing.T) {
	root := initRepo(t)

	first, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.NoError(t, err)

	second, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.NoError(t, err, "second call must be idempotent, not error")
	require.Equal(t, first.Branch, second.Branch)
	require.Equal(t, first.Path, second.Path)
}

// TestCreate_AttachesWorktreeWhenBranchExistsButWorktreeMissing asserts the
// branch-without-worktree recovery path: a prior dispatch crashed after `git
// branch` but before `git worktree add`, or the user manually pruned the
// worktree. Create should attach a fresh worktree to the existing branch.
func TestCreate_AttachesWorktreeWhenBranchExistsButWorktreeMissing(t *testing.T) {
	root := initRepo(t)

	first, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.NoError(t, err)

	// Drop the worktree (force, to remove without confirmation) but keep the branch.
	out, err := exec.Command("git", "-C", root, "worktree", "remove", "--force", first.Path).CombinedOutput()
	require.NoError(t, err, "worktree remove: %s", string(out))

	// Branch must still exist.
	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", first.Branch).Output()
	require.NoError(t, err)
	require.Contains(t, string(branchOut), first.Branch)

	res, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.NoError(t, err)
	require.Equal(t, first.Branch, res.Branch)
	require.Equal(t, first.Path, res.Path)
	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

// TestCreate_DifferentPathConflictRefuses asserts that if the branch is
// already checked out at a non-canonical path (user mucked with worktrees
// manually), Create refuses rather than silently masking the divergence.
func TestCreate_DifferentPathConflictRefuses(t *testing.T) {
	root := initRepo(t)

	first, err := Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.NoError(t, err)

	// Move the worktree somewhere unexpected.
	movedTo := filepath.Join(t.TempDir(), "elsewhere")
	out, err := exec.Command("git", "-C", root, "worktree", "move", first.Path, movedTo).CombinedOutput()
	require.NoError(t, err, "worktree move: %s", string(out))

	_, err = Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already checked out")
}

// TestCreate_CleansUpBranchOnWorktreeFailureForFreshBranch asserts that when
// Create has to create the branch itself and `git worktree add` then fails,
// the orphan branch is deleted — preserving the "no half-created state"
// invariant. Pre-existing branches are NOT deleted (covered by
// TestCreate_AttachesWorktreeWhenBranchExistsButWorktreeMissing).
func TestCreate_CleansUpBranchOnWorktreeFailureForFreshBranch(t *testing.T) {
	root := initRepo(t)

	// Force `git worktree add` to fail by occupying the canonical path with a
	// non-empty regular file before Create runs.
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	require.NoError(t, err)
	canonical := filepath.Join(parent, "repo-wt", "auth", "1-setup")
	require.NoError(t, os.MkdirAll(filepath.Dir(canonical), 0o755))
	require.NoError(t, os.WriteFile(canonical, []byte("blocker"), 0o644))

	_, err = Create(context.Background(), config.Config{}, root, "auth", 1, "setup", "")
	require.Error(t, err)

	branch := "tester/auth--1-setup"
	out, err := exec.Command("git", "-C", root, "branch", "--list", branch).Output()
	require.NoError(t, err)
	require.NotContains(t, string(out), branch, "fresh branch must be cleaned up after worktree-add failure")
}

func TestGC_RemovesMatchingSlug(t *testing.T) {
	root := initRepo(t)

	a, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "")
	require.NoError(t, err)
	b, err := Create(context.Background(), config.Config{}, root, "auth", 2, "beta", "")
	require.NoError(t, err)
	c, err := Create(context.Background(), config.Config{}, root, "billing", 1, "charge", "")
	require.NoError(t, err)

	safe := map[string]bool{a.Branch: true, b.Branch: true}
	removed, err := GC(context.Background(), root, "auth", safe, io.Discard, false)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.Path, b.Path}, removed)

	for _, p := range removed {
		_, err := os.Stat(p)
		require.True(t, os.IsNotExist(err), "expected %q removed", p)
	}

	// billing worktree is untouched
	_, err = os.Stat(c.Path)
	require.NoError(t, err)

	// branches deleted
	out, err := exec.Command("git", "-C", root, "branch", "--list").Output()
	require.NoError(t, err)
	require.NotContains(t, string(out), "tester/auth--")
	require.Contains(t, string(out), "tester/billing--")
}

func TestGC_NoMatchesReturnsEmpty(t *testing.T) {
	root := initRepo(t)

	removed, err := GC(context.Background(), root, "ghost", nil, io.Discard, false)
	require.NoError(t, err)
	require.Empty(t, removed)
}

// TestGC_EmptySafeSetPreservesAll asserts that an explicit empty (non-nil)
// safe set blocks every branch from being deleted. This is the dispatcher's
// "no merges succeeded this batch" path — nothing should be GC'd.
func TestGC_EmptySafeSetPreservesAll(t *testing.T) {
	root := initRepo(t)

	a, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "")
	require.NoError(t, err)

	removed, err := GC(context.Background(), root, "auth", map[string]bool{}, io.Discard, false)
	require.NoError(t, err)
	require.Empty(t, removed)

	// Worktree and branch must still exist.
	_, err = os.Stat(a.Path)
	require.NoError(t, err)
	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", a.Branch).Output()
	require.NoError(t, err)
	require.Contains(t, string(branchOut), a.Branch)
}

// TestGC_PreservesUnmergedCommits asserts that even if the caller marks a
// branch safe, GC still uses `branch -d` (not `-D`), so a branch with commits
// not reachable from HEAD survives — protecting against caller mistakes.
func TestGC_PreservesUnmergedCommits(t *testing.T) {
	root := initRepo(t)

	a, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "")
	require.NoError(t, err)

	// Make a commit on the worktree branch that's NOT in the parent's HEAD.
	require.NoError(t, os.WriteFile(filepath.Join(a.Path, "f.txt"), []byte("x"), 0o644))
	for _, args := range [][]string{
		{"-C", a.Path, "add", "f.txt"},
		{"-C", a.Path, "commit", "-m", "wip"},
	} {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	// Caller mistakenly marks it safe.
	removed, err := GC(context.Background(), root, "auth", map[string]bool{a.Branch: true}, io.Discard, false)
	require.NoError(t, err)
	require.Empty(t, removed, "GC must not delete unmerged branch even if marked safe")

	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", a.Branch).Output()
	require.NoError(t, err)
	require.Contains(t, string(branchOut), a.Branch, "branch must survive")
}

// TestGC_IncludeIntegrationRemovesIntegrationWorktree asserts that when
// includeIntegration=true GC removes both the issue worktrees and the
// per-slug integration worktree at {user}/{slug} (no `--` suffix).
func TestGC_IncludeIntegrationRemovesIntegrationWorktree(t *testing.T) {
	root := initRepo(t)

	// Pre-create the integration branch (caller responsibility — dispatch
	// normally handles this via ensureIntegrationBranch).
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	iwt, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	a, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "")
	require.NoError(t, err)

	safe := map[string]bool{a.Branch: true}
	removed, err := GC(context.Background(), root, "auth", safe, io.Discard, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.Path, iwt.Path}, removed)

	_, err = os.Stat(iwt.Path)
	require.True(t, os.IsNotExist(err), "integration worktree must be removed")

	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", iwt.Branch).Output()
	require.NoError(t, err)
	require.NotContains(t, string(branchOut), iwt.Branch, "integration branch must be deleted")
}

// TestGC_WithoutIncludeIntegrationPreservesIntegrationWorktree asserts the
// flag defaults to off: an integration worktree on `{user}/{slug}` (no `--`)
// is left alone when the flag is false. This is the per-batch GC contract
// from MergeBack — only the issue worktrees may be cleaned mid-run.
func TestGC_WithoutIncludeIntegrationPreservesIntegrationWorktree(t *testing.T) {
	root := initRepo(t)

	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	iwt, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	a, err := Create(context.Background(), config.Config{}, root, "auth", 1, "alpha", "")
	require.NoError(t, err)

	safe := map[string]bool{a.Branch: true}
	removed, err := GC(context.Background(), root, "auth", safe, io.Discard, false)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.Path}, removed)

	_, err = os.Stat(iwt.Path)
	require.NoError(t, err, "integration worktree must be preserved")

	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", iwt.Branch).Output()
	require.NoError(t, err)
	require.Contains(t, string(branchOut), iwt.Branch, "integration branch must survive")
}

func TestResolveBase(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	repoRoot := t.TempDir()
	parent, err := filepath.EvalSymlinks(filepath.Dir(repoRoot))
	require.NoError(t, err)

	tests := []struct {
		name    string
		base    string
		want    string
		wantErr bool
	}{
		{
			name: "empty falls back to repo parent (legacy layout)",
			base: "",
			want: parent,
		},
		{
			name: "absolute path used as-is",
			base: "/tmp/plan-bender-worktrees",
			want: "/tmp/plan-bender-worktrees",
		},
		{
			name: "tilde expanded to home directory",
			base: "~/code/wt",
			want: filepath.Join(home, "code", "wt"),
		},
		{
			name: "relative path anchored at repo root",
			base: "./.wt",
			want: filepath.Join(repoRoot, ".wt"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{WorktreeBase: tc.base}
			got, err := resolveBase(cfg, repoRoot)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCreateIntegration_AttachesAtExpectedPath asserts the integration worktree
// lands at {parent}/{repo}-wt/{slug}/_integration on branch {user}/{slug}.
func TestCreateIntegration_AttachesAtExpectedPath(t *testing.T) {
	root := initRepo(t)

	// Pre-create the integration branch (caller responsibility).
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	res, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	require.Equal(t, "tester/auth", res.Branch)
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(parent, "repo-wt", "auth", "_integration"), res.Path)

	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	listOut, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	require.Contains(t, string(listOut), res.Path)
	require.Contains(t, string(listOut), "branch refs/heads/tester/auth")
}

func TestCreateIntegration_IdempotentOnRepeatCall(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	first, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)
	second, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)
	require.Equal(t, first.Path, second.Path)
	require.Equal(t, first.Branch, second.Branch)
}

func TestCreateIntegration_HonorsWorktreeBase(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	custom := t.TempDir()
	res, err := CreateIntegration(context.Background(), root, config.Config{WorktreeBase: custom}, "auth")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(custom, "repo-wt", "auth", "_integration"), res.Path)
}

// TestCreateIntegration_ConflictsWhenBranchCheckedOutElsewhere ensures we don't
// silently produce a second worktree on the same branch in a non-canonical
// location. The user (or a buggy run) might have moved/created one manually;
// surfacing the conflict is preferable to forking history.
func TestCreateIntegration_ConflictsWhenBranchCheckedOutElsewhere(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	elsewhere := filepath.Join(t.TempDir(), "wt-elsewhere")
	out, err = exec.Command("git", "-C", root, "worktree", "add", elsewhere, "tester/auth").CombinedOutput()
	require.NoError(t, err, "git worktree add: %s", string(out))

	_, err = CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.Error(t, err)
	require.Contains(t, err.Error(), "already checked out")
}

// TestCreateIntegration_RecreatesWhenWorktreeDirRemovedOutOfBand asserts recovery
// from a worktree directory that vanished out-of-band (manual rm, evicted tmpdir).
// Git keeps listing the path — branch line and all — flagged `prunable`, so the
// idempotency check would otherwise hand back a stale path that ResetIntegration's
// `git -C <missing>` aborts on. CreateIntegration must prune the stale entry and
// recreate the directory.
func TestCreateIntegration_RecreatesWhenWorktreeDirRemovedOutOfBand(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	first, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	// Remove the directory WITHOUT `git worktree remove`, leaving git's metadata
	// pointing at a now-missing path (the prunable state).
	require.NoError(t, os.RemoveAll(first.Path))

	res, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err, "must recover from a pruned worktree dir, not hand back a stale path")
	require.Equal(t, first.Path, res.Path)
	require.Equal(t, first.Branch, res.Branch)

	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir(), "directory must be recreated on disk")

	// The recovered worktree must be usable by the very call that would have crashed.
	require.NoError(t, ResetIntegration(context.Background(), res.Path, res.Branch))
}

// TestResetIntegration_AbortsInFlightMergeAndCleansDirtyTree asserts ResetIntegration
// recovers a worktree pre-seeded with MERGE_HEAD + an untracked file. The merge
// state goes away and the worktree is hard-reset to branch's tip.
func TestResetIntegration_AbortsInFlightMergeAndCleansDirtyTree(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	iwt, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	// Resolve the worktree's git dir to write MERGE_HEAD.
	gitDirOut, err := exec.Command("git", "-C", iwt.Path, "rev-parse", "--git-dir").Output()
	require.NoError(t, err)
	gitDir := strings.TrimSpace(string(gitDirOut))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(iwt.Path, gitDir)
	}

	// A real-ish merge head: point it at HEAD itself.
	headSHA, err := exec.Command("git", "-C", iwt.Path, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), headSHA, 0o644))

	// Untracked dirty file in the worktree.
	require.NoError(t, os.WriteFile(filepath.Join(iwt.Path, "dirty.txt"), []byte("garbage\n"), 0o644))

	require.NoError(t, ResetIntegration(context.Background(), iwt.Path, iwt.Branch))

	_, statErr := os.Stat(filepath.Join(gitDir, "MERGE_HEAD"))
	require.True(t, os.IsNotExist(statErr), "MERGE_HEAD must be cleared")

	_, statErr = os.Stat(filepath.Join(iwt.Path, "dirty.txt"))
	require.True(t, os.IsNotExist(statErr), "untracked file must be cleaned")

	// Worktree must still be on the integration branch.
	sym, err := exec.Command("git", "-C", iwt.Path, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	require.Equal(t, iwt.Branch, strings.TrimSpace(string(sym)))
}

// TestResetIntegration_NoOpOnFreshWorktree ensures ResetIntegration on a clean
// worktree (no MERGE_HEAD, nothing dirty) returns nil without error. The
// `git merge --abort` path is silently ignored.
func TestResetIntegration_NoOpOnFreshWorktree(t *testing.T) {
	root := initRepo(t)
	out, err := exec.Command("git", "-C", root, "branch", "tester/auth").CombinedOutput()
	require.NoError(t, err, "git branch: %s", string(out))

	iwt, err := CreateIntegration(context.Background(), root, config.Config{}, "auth")
	require.NoError(t, err)

	require.NoError(t, ResetIntegration(context.Background(), iwt.Path, iwt.Branch))
}

func TestCreate_ReturnsErrorWhenGitMissing(t *testing.T) {
	if !filepath.IsAbs(t.TempDir()) {
		t.Skip("expects absolute tempdir")
	}
	root := t.TempDir() // not a git repo

	_, err := Create(context.Background(), config.Config{}, root, "auth", 1, "x", "")
	require.Error(t, err)
	// Any of the error-wrapping prefixes is acceptable — we're asserting Create
	// surfaces a recognizable failure, not pinning the exact failure point.
	msg := err.Error()
	matched := strings.Contains(msg, "git") || strings.Contains(msg, "user") ||
		strings.Contains(msg, "branch") || strings.Contains(msg, "worktree") ||
		strings.Contains(msg, "not a git repository")
	require.True(t, matched, "expected git-related error, got: %s", msg)
}
