package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const statusPrdYAML = `name: Ship It
slug: ship
status: active
`

const statusIssueOneDoneYAML = `id: 1
slug: setup-config
name: Setup config
track: intent
status: done
priority: high
points: 1
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2026-05-01"
tdd: true
outcome: Configured
scope: Small
acceptance_criteria: ["It works"]
steps: ["Target — works"]
use_cases: ["UC-1"]
`

const statusIssueTwoBlockedYAML = `id: 2
slug: add-middleware
name: Add middleware
track: intent
status: blocked
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2026-05-01"
tdd: true
outcome: Middleware
scope: Small
acceptance_criteria: ["It works"]
steps: ["Target — works"]
use_cases: ["UC-1"]
notes: |
  subprocess timed out after 30m

  follow-up failure detail
`

const statusIssueThreeTodoYAML = `id: 3
slug: deploy
name: Deploy it
track: intent
status: todo
priority: medium
points: 1
labels: [HITL]
blocked_by: [2]
blocking: []
created: "2026-04-30"
updated: "2026-05-01"
tdd: true
outcome: Deployed
scope: Small
acceptance_criteria: ["It works"]
steps: ["Target — works"]
use_cases: ["UC-1"]
`

func setupStatusPlan(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	plansDir := filepath.Join(dir, ".plan-bender", "plans", "ship")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "issues"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.yaml"), []byte(statusPrdYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "1-setup-config.yaml"), []byte(statusIssueOneDoneYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "2-add-middleware.yaml"), []byte(statusIssueTwoBlockedYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "3-deploy.yaml"), []byte(statusIssueThreeTodoYAML), 0o644))
	return dir
}

func TestStatus_HumanOutput_SummarizesAndShowsBlockedReason(t *testing.T) {
	setupStatusPlan(t)

	cmd := NewStatusCmd()
	cmd.SetArgs([]string{"ship"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "Plan: ship")
	assert.Contains(t, output, "Ship It")
	assert.Contains(t, output, "3 issues")
	assert.Contains(t, output, "1 todo")
	assert.Contains(t, output, "1 blocked")
	assert.Contains(t, output, "1 done")

	assert.Contains(t, output, "#1")
	assert.Contains(t, output, "#2")
	assert.Contains(t, output, "#3")

	assert.Contains(t, output, "[AFK]")
	assert.Contains(t, output, "[HITL]")
	assert.Contains(t, output, "blocked_by: #2")

	// Notes truncated to first line, not the multi-line full body.
	assert.Contains(t, output, "subprocess timed out after 30m")
	assert.NotContains(t, output, "follow-up failure detail")
}

func TestStatus_AgentModeJSON_ContainsFullNotesAndShape(t *testing.T) {
	setupStatusPlan(t)

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"status", "ship"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	var got statusJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))

	assert.Equal(t, "ship", got.Plan.Slug)
	assert.Equal(t, "Ship It", got.Plan.Name)
	assert.Equal(t, 3, got.Plan.Total)
	assert.Equal(t, 1, got.Plan.ByStatus["done"])
	assert.Equal(t, 1, got.Plan.ByStatus["blocked"])
	assert.Equal(t, 1, got.Plan.ByStatus["todo"])

	require.Len(t, got.Issues, 3)
	assert.Equal(t, 1, got.Issues[0].ID)
	assert.Equal(t, 2, got.Issues[1].ID)
	assert.Equal(t, 3, got.Issues[2].ID)

	require.NotNil(t, got.Issues[1].Notes)
	assert.Contains(t, *got.Issues[1].Notes, "subprocess timed out")
	assert.Contains(t, *got.Issues[1].Notes, "follow-up failure detail")

	assert.Equal(t, []int{2}, got.Issues[2].BlockedBy)
}

func TestStatus_UnknownPlan(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	cmd := NewStatusCmd()
	cmd.SetArgs([]string{"ghost"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, ErrPlanNotFound, agentErr.Code)
}
