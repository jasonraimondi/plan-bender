package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalPRD = `name: Test
slug: test
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: T
why: T
outcome: T
`

const issueTodoHigh = `id: 1
slug: first
name: First
track: intent
status: todo
priority: high
points: 1
labels: [AFK]
blocked_by: []
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: out
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`

const issueDone = `id: 2
slug: done-one
name: Done One
track: intent
status: done
priority: high
points: 1
labels: [AFK]
blocked_by: []
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: out
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`

func setupPlan(t *testing.T, slug string, issueFiles map[string]string) {
	t.Helper()
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".plan-bender", "plans", slug)
	issuesDir := filepath.Join(plansDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.yaml"), []byte(minimalPRD), 0o644))
	for name, body := range issueFiles {
		require.NoError(t, os.WriteFile(filepath.Join(issuesDir, name), []byte(body), 0o644))
	}
	require.NoError(t, os.Chdir(dir))
}

func TestNextCmd_AgentMode_EmitsJSON(t *testing.T) {
	setupPlan(t, "test", map[string]string{
		"1.yaml": issueTodoHigh,
	})

	root := NewAgentRootCmd("test")
	root.AddCommand(NewNextCmd())
	root.SetArgs([]string{"next", "test"})
	var out strings.Builder
	root.SetOut(&out)

	require.NoError(t, root.Execute())

	var got plan.Result
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	require.NotNil(t, got.Issue)
	assert.Equal(t, 1, got.Issue.ID)
	assert.False(t, got.AllDone)
}

func TestNextCmd_HumanMode_EmitsText(t *testing.T) {
	setupPlan(t, "test", map[string]string{
		"1.yaml": issueTodoHigh,
	})

	cmd := NewNextCmd()
	cmd.SetArgs([]string{"test"})
	var out strings.Builder
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())

	text := out.String()
	assert.Contains(t, text, "#1")
	assert.Contains(t, text, "First")
	assert.Contains(t, text, "reason:")

	var maybeJSON plan.Result
	assert.Error(t, json.Unmarshal([]byte(text), &maybeJSON), "human mode must not emit JSON")
}

func TestNextCmd_AllDone_AgentMode(t *testing.T) {
	setupPlan(t, "test", map[string]string{
		"2.yaml": issueDone,
	})

	root := NewAgentRootCmd("test")
	root.AddCommand(NewNextCmd())
	root.SetArgs([]string{"next", "test"})
	var out strings.Builder
	root.SetOut(&out)

	require.NoError(t, root.Execute(), "exit 0 expected when all done")

	var got plan.Result
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	assert.Nil(t, got.Issue)
	assert.True(t, got.AllDone)
}

func TestNextCmd_AllDone_HumanMode(t *testing.T) {
	setupPlan(t, "test", map[string]string{
		"2.yaml": issueDone,
	})

	cmd := NewNextCmd()
	cmd.SetArgs([]string{"test"})
	var out strings.Builder
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "All issues done")
}

func TestNextCmd_PlanNotFound_ReturnsAgentError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))
	require.NoError(t, os.Chdir(dir))

	cmd := NewNextCmd()
	cmd.SetArgs([]string{"missing"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)

	var agentErr *AgentError
	require.True(t, errors.As(err, &agentErr))
	assert.Equal(t, ErrPlanNotFound, agentErr.Code)
}
