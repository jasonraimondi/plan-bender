package dispatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/status"
)

// makeMergeableBranch creates a branch off integration with one new committed
// file so MergeBack's `git merge --no-ff` succeeds without conflict. It leaves
// the parent worktree checked out on integrationBranch with a clean state.
func makeMergeableBranch(t *testing.T, root, integrationBranch, branch, file string) {
	t.Helper()
	for _, args := range [][]string{
		{"checkout", integrationBranch},
		{"branch", branch},
		{"checkout", branch},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, file), []byte("hi\n"), 0o644))
	for _, args := range [][]string{
		{"add", file},
		{"commit", "-m", "feature"},
		{"checkout", integrationBranch},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
}

// TestDispatcher_MergeBack_CASMismatchSurfaces drives MergeBack against an
// issue YAML whose status is not in the from-set [in-review] and not equal to
// the target done. owner.Transition returns *ErrCASMismatch; MergeBack must
// wrap and return it instead of swallowing.
func TestDispatcher_MergeBack_CASMismatchSurfaces(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", fix.root, "checkout", integrationBranch).CombinedOutput()
	require.NoError(t, err, "checkout: %s", string(out))

	makeMergeableBranch(t, fix.root, integrationBranch, "feat/1-alpha", "alpha.txt")

	results := []SubResult{{IssueID: 1, Success: true, Branch: "feat/1-alpha"}}
	err = d.MergeBack(context.Background(), "demo", results, integrationBranch)

	require.Error(t, err)
	var cas *status.ErrCASMismatch
	require.ErrorAs(t, err, &cas, "MergeBack must surface ErrCASMismatch wrapped, not swallow it")
	assert.Equal(t, status.StatusTodo, cas.Current)
}

// TestDispatcher_MergeBack_AlreadyInStateSwallowed drives MergeBack against
// an issue YAML already in done state (crash-recovery: dispatcher restart
// re-issues a transition that already landed). owner.Transition returns
// ErrAlreadyInState; MergeBack must continue silently.
func TestDispatcher_MergeBack_AlreadyInStateSwallowed(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "done"))

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	out, err := exec.Command("git", "-C", fix.root, "checkout", integrationBranch).CombinedOutput()
	require.NoError(t, err, "checkout: %s", string(out))

	makeMergeableBranch(t, fix.root, integrationBranch, "feat/1-alpha", "alpha.txt")

	results := []SubResult{{IssueID: 1, Success: true, Branch: "feat/1-alpha"}}
	err = d.MergeBack(context.Background(), "demo", results, integrationBranch)

	require.NoError(t, err, "ErrAlreadyInState must be swallowed (crash-recovery path)")

	post := loadIssueYAML(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", post.Status, "issue stays done — no spurious write")
}
