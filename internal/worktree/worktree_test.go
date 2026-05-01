package worktree

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

	res, err := Create(context.Background(), root, "auth", 1, "setup-middleware", "")
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

	res, err := Create(context.Background(), root, "auth", 1, "alpha", "integration")
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

func TestCreate_CleansUpBranchOnWorktreeFailure(t *testing.T) {
	root := initRepo(t)

	res, err := Create(context.Background(), root, "auth", 1, "setup", "")
	require.NoError(t, err)

	// Second Create with same id collides on path; branch must be cleaned up
	// so the repo doesn't accumulate orphan branches.
	_, err = Create(context.Background(), root, "auth", 1, "setup", "")
	require.Error(t, err)

	out, err := exec.Command("git", "-C", root, "branch", "--list", res.Branch).Output()
	require.NoError(t, err)
	// Original branch still exists from successful first call.
	require.Contains(t, string(out), res.Branch)
}

func TestGC_RemovesMatchingSlug(t *testing.T) {
	root := initRepo(t)

	a, err := Create(context.Background(), root, "auth", 1, "alpha", "")
	require.NoError(t, err)
	b, err := Create(context.Background(), root, "auth", 2, "beta", "")
	require.NoError(t, err)
	c, err := Create(context.Background(), root, "billing", 1, "charge", "")
	require.NoError(t, err)

	safe := map[string]bool{a.Branch: true, b.Branch: true}
	removed, err := GC(context.Background(), root, "auth", safe, io.Discard)
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

	removed, err := GC(context.Background(), root, "ghost", nil, io.Discard)
	require.NoError(t, err)
	require.Empty(t, removed)
}

// TestGC_EmptySafeSetPreservesAll asserts that an explicit empty (non-nil)
// safe set blocks every branch from being deleted. This is the dispatcher's
// "no merges succeeded this batch" path — nothing should be GC'd.
func TestGC_EmptySafeSetPreservesAll(t *testing.T) {
	root := initRepo(t)

	a, err := Create(context.Background(), root, "auth", 1, "alpha", "")
	require.NoError(t, err)

	removed, err := GC(context.Background(), root, "auth", map[string]bool{}, io.Discard)
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

	a, err := Create(context.Background(), root, "auth", 1, "alpha", "")
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
	removed, err := GC(context.Background(), root, "auth", map[string]bool{a.Branch: true}, io.Discard)
	require.NoError(t, err)
	require.Empty(t, removed, "GC must not delete unmerged branch even if marked safe")

	branchOut, err := exec.Command("git", "-C", root, "branch", "--list", a.Branch).Output()
	require.NoError(t, err)
	require.Contains(t, string(branchOut), a.Branch, "branch must survive")
}

func TestCreate_ReturnsErrorWhenGitMissing(t *testing.T) {
	if !filepath.IsAbs(t.TempDir()) {
		t.Skip("expects absolute tempdir")
	}
	root := t.TempDir() // not a git repo

	_, err := Create(context.Background(), root, "auth", 1, "x", "")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "git") || strings.Contains(err.Error(), "user"))
}
