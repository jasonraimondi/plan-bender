package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePlan(t *testing.T, plansDir, slug string, prd string, issues map[string]string) {
	t.Helper()
	planDir := filepath.Join(plansDir, slug)
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))
	for name, body := range issues {
		require.NoError(t, os.WriteFile(filepath.Join(issuesDir, name), []byte(body), 0o644))
	}
}

func TestLoadPrd(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	prdYAML := `name: Test Plan
slug: test-plan
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A test
why: Testing
outcome: Tests pass
`
	writePlan(t, plansDir, "test-plan", prdYAML, nil)

	prd, err := LoadPrd(plansDir, "test-plan")
	require.NoError(t, err)
	assert.Equal(t, "Test Plan", prd.Name)
	assert.Equal(t, "active", prd.Status)
}

func TestLoadPrd_NotFound(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	_, err := LoadPrd(plansDir, "nonexistent")
	require.Error(t, err)
}

func TestLoadIssues(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

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
	writePlan(t, plansDir, "test-plan", "name: x\n", map[string]string{
		"001-first.yaml":  issue1,
		"002-second.yaml": issue2,
	})

	issues, err := LoadIssues(plansDir, "test-plan")
	require.NoError(t, err)
	require.Len(t, issues, 2)
	assert.Equal(t, 1, issues[0].ID)
	assert.Equal(t, 2, issues[1].ID)
}

func TestLoadIssues_NotFound(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	_, err := LoadIssues(plansDir, "nonexistent")
	require.Error(t, err)
}

func TestIssueStats(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Status: "done", Points: 2},
		{ID: 2, Status: "in-progress", Points: 3},
		{ID: 3, Status: "blocked", Points: 1},
		{ID: 4, Status: "done", Points: 2},
	}

	stats := IssueStats(issues)

	assert.Equal(t, 4, stats.Total)
	assert.Equal(t, 2, stats.Done)
	assert.Equal(t, 8, stats.TotalPoints)
	assert.Equal(t, 4, stats.DonePoints)
	assert.Equal(t, 1, stats.Blocked)
}

func TestIssueStats_Empty(t *testing.T) {
	stats := IssueStats(nil)
	assert.Equal(t, Stats{}, stats)
}

func TestBuildGraphJSON(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Name: "First", Status: "done", BlockedBy: []int{}},
		{ID: 2, Name: "Second", Status: "in-progress", BlockedBy: []int{1}},
		{ID: 3, Name: "Third", Status: "backlog", BlockedBy: []int{1, 2}},
	}

	g := BuildGraphJSON(issues)

	require.Len(t, g.Nodes, 3)
	assert.Equal(t, 1, g.Nodes[0].ID)
	assert.Equal(t, "First", g.Nodes[0].Name)
	assert.Equal(t, "done", g.Nodes[0].Status)

	require.Len(t, g.Edges, 3)
	assert.Equal(t, GraphEdge{From: 1, To: 2}, g.Edges[0])
	assert.Equal(t, GraphEdge{From: 1, To: 3}, g.Edges[1])
	assert.Equal(t, GraphEdge{From: 2, To: 3}, g.Edges[2])
}

func TestBuildGraphJSON_Empty(t *testing.T) {
	g := BuildGraphJSON(nil)

	assert.Empty(t, g.Nodes)
	assert.Empty(t, g.Edges)
}

func TestListPlans(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	prd1 := `name: Plan Alpha
slug: alpha
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A
why: A
outcome: A
`
	prd2 := `name: Plan Beta
slug: beta
status: draft
created: "2025-01-01"
updated: "2025-01-02"
description: B
why: B
outcome: B
`
	writePlan(t, plansDir, "alpha", prd1, nil)
	writePlan(t, plansDir, "beta", prd2, nil)

	plans, err := ListPlans(plansDir)
	require.NoError(t, err)
	require.Len(t, plans, 2)

	// Should be sorted by directory name (alpha before beta)
	slugs := []string{plans[0].Slug, plans[1].Slug}
	assert.Contains(t, slugs, "alpha")
	assert.Contains(t, slugs, "beta")
}

func TestListPlans_EmptyDir(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	plans, err := ListPlans(plansDir)
	require.NoError(t, err)
	assert.Empty(t, plans)
}

func TestListPlans_NonexistentDir(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	plans, err := ListPlans(plansDir)
	require.NoError(t, err)
	assert.Empty(t, plans)
}
