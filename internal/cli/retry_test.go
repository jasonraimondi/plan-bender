package cli

import (
	"encoding/json"
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

const retryIssueYAML = `id: 4
slug: ship-cli
name: Ship the CLI
track: intent
status: blocked
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2024-01-01"
tdd: true
outcome: Shipped
scope: Small
acceptance_criteria: ["It ships"]
steps: ["Target — ships"]
use_cases: ["UC-1"]
notes: subprocess timed out after 30m
`

func setupRetryPlan(t *testing.T, status string, withNotes bool) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	plansDir := filepath.Join(dir, ".plan-bender", "plans", "ship")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.yaml"), []byte("name: Ship\nslug: ship\nstatus: active\n"), 0o644))

	body := retryIssueYAML
	if status != "" {
		body = strings.Replace(body, "status: blocked", "status: "+status, 1)
	}
	if !withNotes {
		body = strings.Replace(body, "notes: subprocess timed out after 30m\n", "", 1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "4-ship-cli.yaml"), []byte(body), 0o644))
	return dir
}

func loadRetryIssue(t *testing.T, dir string) schema.IssueYaml {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "ship", "issues", "4-ship-cli.yaml"))
	require.NoError(t, err)
	var issue schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &issue))
	return issue
}

func TestRetry_FlipsBlockedToTodoAndClearsNotes(t *testing.T) {
	dir := setupRetryPlan(t, "", true)

	cmd := NewRetryCmd()
	cmd.SetArgs([]string{"ship", "4"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "blocked → todo")
	assert.Contains(t, out.String(), "cleared notes")

	issue := loadRetryIssue(t, dir)
	assert.Equal(t, "todo", issue.Status)
	assert.Nil(t, issue.Notes, "notes should be cleared")
	assert.Equal(t, time.Now().Format("2006-01-02"), issue.Updated)
}

func TestRetry_RefusesNonBlocked(t *testing.T) {
	for _, status := range []string{"todo", "in-progress", "in-review", "done", "canceled"} {
		t.Run(status, func(t *testing.T) {
			dir := setupRetryPlan(t, status, true)

			cmd := NewRetryCmd()
			cmd.SetArgs([]string{"ship", "4"})
			var out, errOut strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			err := cmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not blocked")

			issue := loadRetryIssue(t, dir)
			assert.Equal(t, status, issue.Status, "status should not change on refusal")
			require.NotNil(t, issue.Notes, "notes should not be cleared on refusal")
		})
	}
}

func TestRetry_UnknownIssue(t *testing.T) {
	setupRetryPlan(t, "", true)

	cmd := NewRetryCmd()
	cmd.SetArgs([]string{"ship", "99"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, ErrPlanNotFound, agentErr.Code)
}

func TestRetry_AgentModeJSON(t *testing.T) {
	dir := setupRetryPlan(t, "", true)

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"retry", "ship", "4"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	assert.Equal(t, "ok", got["status"])
	assert.EqualValues(t, 4, got["id"])
	assert.Equal(t, "todo", got["new_status"])
	assert.Equal(t, "subprocess timed out after 30m", got["cleared_notes"])

	issue := loadRetryIssue(t, dir)
	assert.Equal(t, "todo", issue.Status)
	assert.Nil(t, issue.Notes)
}
