package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/dispatch"
)

func setupDispatchCLI(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# r\n"), 0o644))
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender.json"),
		[]byte(`{"plans_dir": "./.plan-bender/plans/", "agents": {"claude-code": true}}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".plan-bender", "plans", "demo", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender", "plans", "demo", "prd.json"),
		[]byte(validDemoPrd), 0o644))
	chdir(t, root)
	return root
}

const dispatchCLIIssue = `{
  "id": 1,
  "slug": "alpha",
  "name": "Alpha",
  "track": "intent",
  "status": %q,
  "priority": "high",
  "points": 1,
  "labels": [%q],
  "assignee": null,
  "blocked_by": [],
  "blocking": [],
  "branch": null,
  "pr": null,
  "linear_id": null,
  "created": "2026-04-30",
  "updated": "2026-04-30",
  "tdd": true,
  "outcome": "out",
  "scope": "scope",
  "acceptance_criteria": ["ok"],
  "steps": ["x — y"],
  "use_cases": ["UC-1"]
}`

func writeDispatchCLIIssue(t *testing.T, root, status, label string) {
	t.Helper()
	body := fmt.Sprintf(dispatchCLIIssue, status, label)
	path := filepath.Join(root, ".plan-bender", "plans", "demo", "issues", "1-alpha.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestDispatchCmd_AllDoneExitsZero(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "done", "AFK")

	cmd := NewDispatchCmd("test")
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	require.NoError(t, cmd.Execute())
}

func TestDispatchCmd_HITLOnlyReturnsHITLError(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "todo", "HITL")

	cmd := NewDispatchCmd("test")
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, IsHITLOnly(err), "expected IsHITLOnly to recognize the error, got %v", err)
	assert.Contains(t, out.String(), "HITL")
}

func TestIsHITLOnly_RecognizesWrappedError(t *testing.T) {
	wrapped := fmt.Errorf("dispatch failed: %w", dispatch.ErrHITLOnly)
	assert.True(t, IsHITLOnly(wrapped))

	other := errors.New("something else")
	assert.False(t, IsHITLOnly(other))
}

func TestDispatchCmd_UnknownPlanReturnsError(t *testing.T) {
	setupDispatchCLI(t)

	cmd := NewDispatchCmd("test")
	cmd.SetArgs([]string{"ghost"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.False(t, IsHITLOnly(err), "unknown plan must not be confused with HITL")
}

func TestDispatchCmd_InvalidBaseErrors(t *testing.T) {
	setupDispatchCLI(t)
	writeDispatchCLIIssue(t, ".", "done", "AFK")

	cmd := NewDispatchCmd("test")
	cmd.SetArgs([]string{"demo", "--base", "does-not-exist"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

// All-done plan short-circuits the loop so no claude stub is needed.
func TestDispatchCmd_ValidBaseAccepted(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "done", "AFK")

	cmd := NewDispatchCmd("test")
	cmd.SetArgs([]string{"demo", "--base", "main"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	require.NoError(t, cmd.Execute())
}

func bugReports(t *testing.T, root string) []string {
	t.Helper()
	reports, err := filepath.Glob(filepath.Join(root, "pb-error-report-*.log"))
	require.NoError(t, err)
	return reports
}

// A todo AFK issue with no bender-implement-issue skill staged fails setup for
// every ready issue, so dispatch returns an environment error in Go — the path
// the agent-facing report_bugs prompt can't cover. With report_bugs on, the
// command must leave the artifact itself.
func TestDispatchCmd_WritesBugReportOnFailureWhenReportBugs(t *testing.T) {
	root := setupDispatchCLI(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender.json"),
		[]byte(`{"plans_dir": "./.plan-bender/plans/", "agents": {"claude-code": true}, "report_bugs": true}`), 0o644))
	writeDispatchCLIIssue(t, root, "todo", "AFK")

	cmd := NewDispatchCmd("v1.2.3")
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	require.Error(t, cmd.Execute())

	reports := bugReports(t, root)
	require.Len(t, reports, 1, "exactly one bug report should be written")
	data, err := os.ReadFile(reports[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), "dispatch demo")
	assert.Contains(t, string(data), "v1.2.3")
}

// The same failure with report_bugs off (the default) must not write a report —
// proving the flag gates the artifact rather than any failure producing one.
func TestDispatchCmd_NoBugReportWhenReportBugsOff(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "todo", "AFK")

	cmd := NewDispatchCmd("v1.2.3")
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	require.Error(t, cmd.Execute())
	assert.Empty(t, bugReports(t, root), "no report when report_bugs is off")
}

func TestDispatchCmd_NoBugReportOnStuckBlocked(t *testing.T) {
	root := setupDispatchCLI(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender.json"),
		[]byte(`{"plans_dir": "./.plan-bender/plans/", "agents": {"claude-code": true}, "report_bugs": true}`), 0o644))
	writeDispatchCLIIssue(t, root, "blocked", "AFK")

	cmd := NewDispatchCmd("v1.2.3")
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.False(t, IsSetupFailure(err), "stuck-on-blocked must not be the setup-failure class")
	assert.Empty(t, bugReports(t, root), "no report for a user-resolvable stuck state")
}
