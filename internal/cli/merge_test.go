package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMerge_AgentModeEmptyArraysNotNull asserts the agent-mode JSON encodes the
// empty no-op result as `[]`, not `null` — agent consumers index the arrays, so
// a null would force every caller to nil-check. The seeded issue is in-progress
// (not in-review), so Merge is a no-op and exercises the intsOrEmpty mapping.
func TestMerge_AgentModeEmptyArraysNotNull(t *testing.T) {
	setupCompletePlan(t, "in-progress")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"merge", "ship"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	var resp struct {
		Merged     []int `json:"merged"`
		Conflicted []int `json:"conflicted"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()), &resp))
	assert.NotNil(t, resp.Merged, "merged must encode as [] not null")
	assert.NotNil(t, resp.Conflicted, "conflicted must encode as [] not null")
	assert.Empty(t, resp.Merged)
	assert.Empty(t, resp.Conflicted)
}

// TestMerge_HumanModeNoOpMessage asserts the human path prints a readable
// "nothing to merge" line when no issue is in-review.
func TestMerge_HumanModeNoOpMessage(t *testing.T) {
	setupCompletePlan(t, "todo")

	cmd := NewMergeCmd()
	cmd.SetArgs([]string{"ship"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "nothing to merge")
}
