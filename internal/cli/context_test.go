package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupContextTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")

	planDir := filepath.Join(plansDir, "test-plan")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	prd := `name: Test Plan
slug: test-plan
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A test plan
why: Testing
outcome: Tests pass
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))

	issue1 := `id: 1
slug: first
name: First Issue
track: intent
status: done
priority: high
points: 2
labels: []
blocked_by: []
blocking: [2]
created: "2025-01-01"
updated: "2025-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`
	issue2 := `id: 2
slug: second
name: Second Issue
track: experience
status: in-progress
priority: medium
points: 3
labels: []
blocked_by: [1]
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "001-first.yaml"), []byte(issue1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "002-second.yaml"), []byte(issue2), 0o644))

	require.NoError(t, os.Chdir(dir))
	return dir
}

func TestContextCmd_NoSlug_ListsPlans(t *testing.T) {
	setupContextTestDir(t)

	cmd := NewContextCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	var summaries []plan.PlanSummary
	require.NoError(t, json.Unmarshal([]byte(out.String()), &summaries))

	require.Len(t, summaries, 1)
	assert.Equal(t, "test-plan", summaries[0].Slug)
	assert.Equal(t, "Test Plan", summaries[0].Name)
	assert.Equal(t, "active", summaries[0].Status)
	assert.Equal(t, 2, summaries[0].Issues)
	assert.Equal(t, 1, summaries[0].Done)
	assert.Equal(t, 5, summaries[0].Points)
	assert.Equal(t, 0, summaries[0].Blocked)
}

func TestContextCmd_WithSlug_ReturnsFullContext(t *testing.T) {
	setupContextTestDir(t)

	cmd := NewContextCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"test-plan"})
	require.NoError(t, cmd.Execute())

	var ctx contextFullJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &ctx))

	// PRD
	require.NotNil(t, ctx.Prd)
	assert.Equal(t, "Test Plan", ctx.Prd.Name)
	assert.Equal(t, "active", ctx.Prd.Status)

	// Issues
	require.Len(t, ctx.Issues, 2)
	assert.Equal(t, 1, ctx.Issues[0].ID)
	assert.Equal(t, 2, ctx.Issues[1].ID)

	// Dependencies
	require.Len(t, ctx.Dependencies.Nodes, 2)
	require.Len(t, ctx.Dependencies.Edges, 1)
	assert.Equal(t, plan.GraphEdge{From: 1, To: 2}, ctx.Dependencies.Edges[0])

	// Stats
	assert.Equal(t, 2, ctx.Stats.Total)
	assert.Equal(t, 1, ctx.Stats.Done)
	assert.Equal(t, 5, ctx.Stats.TotalPoints)
	assert.Equal(t, 0, ctx.Stats.Blocked)
}

func TestContextCmd_NonexistentSlug_ReturnsError(t *testing.T) {
	setupContextTestDir(t)

	root := NewAgentRootCmd("test")
	root.AddCommand(NewContextCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"context", "nonexistent"})

	err := ExecuteAgent(root)
	require.Error(t, err)

	var errResp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &errResp))
	assert.Equal(t, string(ErrPlanNotFound), errResp.Code)
	assert.Contains(t, errResp.Error, "nonexistent")
}

func TestContextCmd_NoSlug_EmptyPlans(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.Chdir(dir))

	cmd := NewContextCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())

	var summaries []plan.PlanSummary
	require.NoError(t, json.Unmarshal([]byte(out.String()), &summaries))
	assert.Empty(t, summaries)
}

func TestContextFullJSON_MarshalShape(t *testing.T) {
	ctx := contextFullJSON{
		Prd:    &schema.PrdYaml{Name: "Test"},
		Issues: []schema.IssueYaml{{ID: 1}},
		Dependencies: plan.Graph{
			Nodes: []plan.GraphNode{{ID: 1, Name: "A", Status: "done"}},
			Edges: []plan.GraphEdge{{From: 1, To: 2}},
		},
		Stats: plan.Stats{Total: 1, Done: 1, TotalPoints: 2, DonePoints: 2},
	}

	data, err := json.Marshal(ctx)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "prd")
	assert.Contains(t, raw, "issues")
	assert.Contains(t, raw, "dependencies")
	assert.Contains(t, raw, "stats")
}
