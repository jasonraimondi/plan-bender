package backend

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/stretchr/testify/assert"
)

func TestMapPriority(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"urgent", 1},
		{"high", 2},
		{"medium", 3},
		{"low", 4},
		{"unknown", 3}, // default
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, mapPriority(tt.in), "mapPriority(%q)", tt.in)
	}
}

func TestReversePriority(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{1, "urgent"},
		{2, "high"},
		{3, "medium"},
		{4, "low"},
		{0, "medium"},  // default
		{99, "medium"}, // default
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ReversePriority(tt.in), "ReversePriority(%d)", tt.in)
	}
}

func TestResolveStateID(t *testing.T) {
	b := &linearBackend{
		cfg: func() config.Config {
			c := config.Defaults()
			c.Linear.StatusMap = map[string]string{"in-progress": "In Progress"}
			return c
		}(),
		stateIDs: map[string]string{
			"In Progress": "state-1",
			"Backlog":     "state-2",
			"Done":        "state-3",
		},
	}

	// status_map hit
	assert.Equal(t, "state-1", b.resolveStateID("in-progress"))

	// Case-insensitive fallback
	assert.Equal(t, "state-2", b.resolveStateID("backlog"))
	assert.Equal(t, "state-3", b.resolveStateID("done"))

	// No match
	assert.Equal(t, "", b.resolveStateID("nonexistent"))
}

func TestLinearIssueToRemote_WithAssignee(t *testing.T) {
	issue := &linear.Issue{
		ID:       "lin-1",
		Title:    "Test",
		Priority: 2,
		URL:      "https://linear.app/issue/lin-1",
	}
	issue.State.Name = "In Progress"
	issue.Labels.Nodes = []struct{ Name string }{{Name: "bug"}, {Name: "p0"}}
	issue.Assignee = &struct{ Name string }{Name: "alice"}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, "lin-1", remote.ID)
	assert.Equal(t, "Test", remote.Title)
	assert.Equal(t, "In Progress", remote.Status)
	assert.Equal(t, "high", remote.Priority) // Linear 2 → "high"
	assert.Equal(t, []string{"bug", "p0"}, remote.Labels)
	assert.Equal(t, "alice", remote.Assignee)
	assert.Equal(t, "https://linear.app/issue/lin-1", remote.URL)
}

func TestLinearIssueToRemote_NilAssignee(t *testing.T) {
	issue := &linear.Issue{
		ID:       "lin-2",
		Title:    "No Assignee",
		Priority: 0,
	}
	issue.State.Name = "Backlog"

	remote := linearIssueToRemote(issue)
	assert.Equal(t, "", remote.Assignee)
	assert.Equal(t, "medium", remote.Priority) // Linear 0 → "medium" (default)
	assert.Nil(t, remote.Labels)
}

func TestLinearIssueToRemote_MultipleLabels(t *testing.T) {
	issue := &linear.Issue{ID: "lin-3"}
	issue.Labels.Nodes = []struct{ Name string }{
		{Name: "feature"}, {Name: "frontend"}, {Name: "urgent"},
	}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, []string{"feature", "frontend", "urgent"}, remote.Labels)
}
