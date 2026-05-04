package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

const completeIssueYAML = `id: 3
slug: ship-it
name: Ship it
track: intent
status: in-progress
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2024-01-01"
tdd: true
outcome: It ships
scope: Small
acceptance_criteria:
  - It ships
steps:
  - "Target — ships"
use_cases:
  - UC-1
`

func setupCompletePlan(t *testing.T, status string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	plansDir := filepath.Join(dir, ".plan-bender", "plans", "ship")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "issues"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.yaml"), []byte(validShipPrd), 0o644))

	body := completeIssueYAML
	if status != "" {
		body = strings.Replace(body, "status: in-progress", "status: "+status, 1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "3-ship-it.yaml"), []byte(body), 0o644))
	return dir
}

func loadCompleteIssue(t *testing.T, dir string) schema.IssueYaml {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "ship", "issues", "3-ship-it.yaml"))
	require.NoError(t, err)
	var issue schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &issue))
	return issue
}

func TestComplete_FlipsStatusAndEmitsSentinel(t *testing.T) {
	dir := setupCompletePlan(t, "")

	cmd := NewCompleteCmd()
	cmd.SetArgs([]string{"ship", "3"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), `<pba:complete issue-id="3"/>`)

	issue := loadCompleteIssue(t, dir)
	assert.Equal(t, "in-review", issue.Status)
	assert.Equal(t, time.Now().Format("2006-01-02"), issue.Updated, "updated date should be today")
}

// TestComplete_AllowedFromStates exercises every allowed source state for the
// transition (todo, in-progress, backlog) and asserts each lands in in-review.
func TestComplete_AllowedFromStates(t *testing.T) {
	for _, st := range []string{"todo", "in-progress", "backlog"} {
		t.Run(st, func(t *testing.T) {
			dir := setupCompletePlan(t, st)

			cmd := NewCompleteCmd()
			cmd.SetArgs([]string{"ship", "3"})
			var out strings.Builder
			cmd.SetOut(&out)
			require.NoError(t, cmd.Execute())

			assert.Contains(t, out.String(), `<pba:complete issue-id="3"/>`)
			issue := loadCompleteIssue(t, dir)
			assert.Equal(t, "in-review", issue.Status)
		})
	}
}

func TestComplete_IdempotentInReview(t *testing.T) {
	dir := setupCompletePlan(t, "in-review")

	cmd := NewCompleteCmd()
	cmd.SetArgs([]string{"ship", "3"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute(), "re-completing an in-review issue is a no-op success")

	assert.Contains(t, out.String(), `<pba:complete issue-id="3"/>`,
		"sentinel must be emitted so dispatcher detects completion on retry")

	issue := loadCompleteIssue(t, dir)
	assert.Equal(t, "in-review", issue.Status)
}

func TestComplete_RejectsTerminalStates(t *testing.T) {
	for _, st := range []string{"done", "canceled", "blocked"} {
		t.Run(st, func(t *testing.T) {
			dir := setupCompletePlan(t, st)

			cmd := NewCompleteCmd()
			cmd.SetArgs([]string{"ship", "3"})
			var out, errOut strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			err := cmd.Execute()
			require.Error(t, err)

			var agentErr *AgentError
			require.ErrorAs(t, err, &agentErr)
			assert.Equal(t, ErrValidationFailed, agentErr.Code)
			assert.Contains(t, agentErr.Error(), st, "error message reports current state")
			assert.Contains(t, agentErr.Error(), "refusing to overwrite")

			issue := loadCompleteIssue(t, dir)
			assert.Equal(t, st, issue.Status, "status should not change on refusal")
		})
	}
}

func TestComplete_RejectsAlreadyDone(t *testing.T) {
	setupCompletePlan(t, "done")

	cmd := NewCompleteCmd()
	cmd.SetArgs([]string{"ship", "3"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already done")
}

func TestComplete_UnknownPlan(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	cmd := NewCompleteCmd()
	cmd.SetArgs([]string{"ghost", "3"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestComplete_AgentModeJSON(t *testing.T) {
	setupCompletePlan(t, "")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"complete", "ship", "3"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	output := out.String()
	assert.Contains(t, output, `"sentinel"`)
	// JSON encodes < and > as unicode escapes; verify the issue-id appears in the encoded sentinel.
	assert.Contains(t, output, `pba:complete issue-id=\"3\"`)
	assert.Contains(t, output, `"id":3`)
}
