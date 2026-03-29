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

func setupStatusTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")

	// Create a plan with PRD and issues
	planDir := filepath.Join(plansDir, "test-plan")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	prd := `name: Test Plan
slug: test-plan
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A test
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

func TestStatusAllPlans_JSON(t *testing.T) {
	setupStatusTestDir(t)

	cmd := NewStatusCmd()
	cmd.SetArgs([]string{"--json"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	var summaries []planSummaryJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &summaries))

	require.Len(t, summaries, 1)
	assert.Equal(t, "test-plan", summaries[0].Slug)
	assert.Equal(t, "Test Plan", summaries[0].Name)
	assert.Equal(t, "active", summaries[0].Status)
	assert.Equal(t, 2, summaries[0].Issues)
	assert.Equal(t, 1, summaries[0].Done)
	assert.Equal(t, 5, summaries[0].Points)
}

func TestStatusPlanDetail_JSON(t *testing.T) {
	setupStatusTestDir(t)

	cmd := NewStatusCmd()
	cmd.SetArgs([]string{"test-plan", "--json"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	var detail planDetailJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &detail))

	assert.Equal(t, "test-plan", detail.Slug)
	assert.Equal(t, "Test Plan", detail.Name)
	assert.Equal(t, "active", detail.Status)
	assert.NotNil(t, detail.Prd)
	assert.Equal(t, "Test Plan", detail.Prd.Name)
	require.Len(t, detail.Issues, 2)
	assert.Equal(t, 1, detail.Issues[0].ID)
	assert.Equal(t, 2, detail.Issues[1].ID)
}
