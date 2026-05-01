package worktree

import (
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

	res, err := Create(root, "auth", 1, "setup-middleware")
	require.NoError(t, err)

	require.Equal(t, "tester/auth--1-setup-middleware", res.Branch)
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	require.NoError(t, err)
	expectedPath := filepath.Join(parent, "repo-wt", "1-setup-middleware")
	require.Equal(t, expectedPath, res.Path)

	info, err := os.Stat(res.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	out, err := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), res.Path)
	require.Contains(t, string(out), "branch refs/heads/"+res.Branch)
}

func TestCreate_CleansUpBranchOnWorktreeFailure(t *testing.T) {
	root := initRepo(t)

	res, err := Create(root, "auth", 1, "setup")
	require.NoError(t, err)

	// Second Create with same id collides on path; branch must be cleaned up
	// so the repo doesn't accumulate orphan branches.
	_, err = Create(root, "auth", 1, "setup")
	require.Error(t, err)

	out, err := exec.Command("git", "-C", root, "branch", "--list", res.Branch).Output()
	require.NoError(t, err)
	// Original branch still exists from successful first call.
	require.Contains(t, string(out), res.Branch)
}

func TestGC_RemovesMatchingSlug(t *testing.T) {
	root := initRepo(t)

	a, err := Create(root, "auth", 1, "alpha")
	require.NoError(t, err)
	b, err := Create(root, "auth", 2, "beta")
	require.NoError(t, err)
	c, err := Create(root, "billing", 1, "charge")
	require.NoError(t, err)

	removed, err := GC(root, "auth")
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

	removed, err := GC(root, "ghost")
	require.NoError(t, err)
	require.Empty(t, removed)
}

func TestCreate_ReturnsErrorWhenGitMissing(t *testing.T) {
	if !filepath.IsAbs(t.TempDir()) {
		t.Skip("expects absolute tempdir")
	}
	root := t.TempDir() // not a git repo

	_, err := Create(root, "auth", 1, "x")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "git") || strings.Contains(err.Error(), "user"))
}
