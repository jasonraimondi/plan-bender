package cli

import (
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"testing"
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
