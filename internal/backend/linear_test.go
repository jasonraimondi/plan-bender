package backend

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
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
