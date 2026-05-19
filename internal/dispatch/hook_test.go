package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
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

