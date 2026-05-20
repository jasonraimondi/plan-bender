package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

func TestRunHook_EmptyCmdIsNoOp(t *testing.T) {
	var out bytes.Buffer
	stderr, err := RunHook(context.Background(), "", t.TempDir(), &out)
	require.NoError(t, err)
	assert.Empty(t, stderr)
	assert.Empty(t, out.String())
}

func TestRunHook_SuccessStreamsPrefixedOutput(t *testing.T) {
	var out bytes.Buffer
	stderr, err := RunHook(context.Background(), `echo line1; echo line2`, t.TempDir(), &out)
	require.NoError(t, err)
	assert.Empty(t, stderr)
	got := out.String()
	assert.Contains(t, got, "[hook] line1")
	assert.Contains(t, got, "[hook] line2")
}

func TestRunHook_FailureCapturesStderr(t *testing.T) {
	var out bytes.Buffer
	stderr, err := RunHook(context.Background(), `echo boom >&2; exit 1`, t.TempDir(), &out)
	require.Error(t, err)
	assert.Contains(t, stderr, "boom")
	assert.Contains(t, err.Error(), "boom")
}

func TestRunHook_RunsInProvidedDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marker"), []byte("ok\n"), 0o644))
	var out bytes.Buffer
	_, err := RunHook(context.Background(), `cat marker`, dir, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "[hook] ok")
}

// The truncation race: a hook line emitted immediately before exit must still
// reach the prefixed stream. The legacy StdoutPipe + late wg.Wait() pattern
// could lose the tail when cmd.Wait closed the pipe before the reader drained.
// printf (no trailing \n) leaves the tail in linePrefixWriter's partial-line
// buffer so only the post-Wait Flush() can rescue it — exactly the path the
// regression touched.
func TestRunHook_FinalLineBeforeExitNotLost(t *testing.T) {
	var out bytes.Buffer
	_, err := RunHook(context.Background(), `printf FINAL_HOOK_TAIL; exit 0`, t.TempDir(), &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "[hook] FINAL_HOOK_TAIL")
}

// A single hook output line larger than any pipe-read chunk must arrive whole
// behind a single [hook] prefix — not split across multiple prefixed lines.
func TestRunHook_LargeSingleLineNotSplitOrTruncated(t *testing.T) {
	const bigLen = 256 * 1024
	cmd := fmt.Sprintf(`head -c %d /dev/zero | tr '\0' X`, bigLen)
	var out bytes.Buffer
	_, err := RunHook(context.Background(), cmd, t.TempDir(), &out)
	require.NoError(t, err)

	streamed := out.String()
	assert.Contains(t, streamed, "[hook] "+strings.Repeat("X", bigLen),
		"the entire 256KB line must arrive whole behind a single prefix")
	assert.Equal(t, 1, strings.Count(streamed, "[hook] X"),
		"the big line must not be split across multiple prefixed output lines")
}

// A timed-out hook backgrounds a grandchild that inherits the stdout pipe and
// outlives the hook by a wide margin. RunHook must still return on a bounded
// delay: the process-group kill reaps the grandchild and cmd.WaitDelay backstops
// cmd.Wait(). Without either, the stdout-copy goroutine blocks on the surviving
// pipe write end and Wait() hangs for the full grandchild lifetime.
func TestRunHook_TimeoutReturnsDespiteSurvivingGrandchild(t *testing.T) {
	const grandchildLifetime = 60 * time.Second
	cmd := fmt.Sprintf(`sleep %d & sleep %d`,
		int(grandchildLifetime.Seconds()), int(grandchildLifetime.Seconds()))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	var out bytes.Buffer
	start := time.Now()
	_, err := RunHook(ctx, cmd, t.TempDir(), &out)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 30*time.Second,
		"RunHook must return on a bounded delay, not wait out the grandchild lifetime")
}

// Wiring: before_issue hook failure marks the issue blocked and skips the
// subprocess.
func TestDispatcher_BeforeIssueHookFailureBlocksIssue(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)
	// stub claude that would succeed if it ran — we'll prove it didn't.
	installClaudeStub(t, fmt.Sprintf(`sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
exit 0
`, fix.plansDir))

	cfg := config.Defaults()
	cfg.Hooks.BeforeIssue = `echo prep && exit 1`
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir, Out: &bytes.Buffer{}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := d.Run(ctx, "demo")
	require.Error(t, err)

	post := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "blocked", post.Status, "issue should be blocked when before_issue fails")
	require.NotNil(t, post.Notes)
	assert.Contains(t, *post.Notes, "before_issue hook failed")
}

// Wiring: after_issue hook failure does not change issue status.
func TestDispatcher_AfterIssueHookFailureLogsButContinues(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)
	installClaudeStub(t, fmt.Sprintf(`sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
exit 0
`, fix.plansDir))

	cfg := config.Defaults()
	cfg.Hooks.AfterIssue = `echo afterfail >&2; exit 1`
	var out bytes.Buffer
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir, Out: &out}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, d.Run(ctx, "demo"))

	post := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", post.Status, "after_issue hook failure must not block the issue")
	assert.Contains(t, out.String(), "after_issue hook failed")
}

// TestDispatcher_AfterBatchHookCwdIsIntegrationWorktree asserts the after_batch
// hook runs with cwd set to the per-slug integration worktree, NOT the parent
// repo. Hooks that run tests (`pnpm test`) need to see the post-merge state,
// which only exists in the integration worktree under the new MergeBack flow.
func TestDispatcher_AfterBatchHookCwdIsIntegrationWorktree(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)
	installClaudeStub(t, fmt.Sprintf(`sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
exit 0
`, fix.plansDir))

	markerPath := filepath.Join(t.TempDir(), "after_batch_cwd")
	cfg := config.Defaults()
	cfg.Hooks.AfterBatch = fmt.Sprintf(`pwd > %q`, markerPath)
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir, Out: &bytes.Buffer{}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, d.Run(ctx, "demo"))

	data, err := os.ReadFile(markerPath)
	require.NoError(t, err)

	// pwd in the hook may resolve symlinks; EvalSymlinks the expected path to
	// match. The iwt itself is GC'd at AllDone, so we resolve symlinks on the
	// parent dir (which survives) and append the iwt basename rather than
	// EvalSymlinks'ing the leaf path directly.
	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	expected := filepath.Join(parent, "repo-wt", "demo", "_integration")
	gotCwdRaw := strings.TrimSpace(string(data))
	gotParent, err := filepath.EvalSymlinks(filepath.Dir(gotCwdRaw))
	require.NoError(t, err)
	gotCwd := filepath.Join(gotParent, filepath.Base(gotCwdRaw))
	assert.Equal(t, expected, gotCwd,
		"after_batch hook cwd must be the integration worktree path")
}

// Wiring: after_batch hook runs after merge-back; failure logs but does not abort.
func TestDispatcher_AfterBatchHookRuns(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)
	installClaudeStub(t, fmt.Sprintf(`sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
exit 0
`, fix.plansDir))

	markerPath := filepath.Join(t.TempDir(), "after_batch_ran")
	cfg := config.Defaults()
	cfg.Hooks.AfterBatch = fmt.Sprintf(`echo done > %q`, markerPath)
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir, Out: &bytes.Buffer{}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, d.Run(ctx, "demo"))

	data, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	assert.Equal(t, "done\n", string(data))
}

