package plan

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
