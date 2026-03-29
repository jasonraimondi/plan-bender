package cli

import (
	"encoding/json"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMermaidGraph(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Name: "First", Status: "done", BlockedBy: []int{}},
		{ID: 2, Name: "Second", Status: "in-progress", BlockedBy: []int{1}},
		{ID: 3, Name: "Third", Status: "backlog", BlockedBy: []int{1, 2}},
	}

	out := buildMermaidGraph(issues)

	assert.Contains(t, out, "graph TD")
	assert.Contains(t, out, `i1["#1 First"]`)
	assert.Contains(t, out, `i2["#2 Second"]`)
	assert.Contains(t, out, "i1 --> i2")
	assert.Contains(t, out, "i1 --> i3")
	assert.Contains(t, out, "i2 --> i3")
	assert.Contains(t, out, "fill:#2da44e") // done=green
	assert.Contains(t, out, "fill:#bf8700") // in-progress=yellow
	assert.Contains(t, out, "fill:#656d76") // backlog=gray
}

func TestBuildGraphJSON(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Name: "First", Status: "done", BlockedBy: []int{}},
		{ID: 2, Name: "Second", Status: "in-progress", BlockedBy: []int{1}},
		{ID: 3, Name: "Third", Status: "backlog", BlockedBy: []int{1, 2}},
	}

	g := buildGraphJSON(issues)

	data, err := json.Marshal(g)
	require.NoError(t, err)

	var parsed graphJSON
	require.NoError(t, json.Unmarshal(data, &parsed))

	require.Len(t, parsed.Nodes, 3)
	assert.Equal(t, 1, parsed.Nodes[0].ID)
	assert.Equal(t, "First", parsed.Nodes[0].Name)
	assert.Equal(t, "done", parsed.Nodes[0].Status)

	require.Len(t, parsed.Edges, 3)
	assert.Equal(t, graphEdgeJSON{From: 1, To: 2}, parsed.Edges[0])
	assert.Equal(t, graphEdgeJSON{From: 1, To: 3}, parsed.Edges[1])
	assert.Equal(t, graphEdgeJSON{From: 2, To: 3}, parsed.Edges[2])
}
