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

func TestAgentValidate_ValidPlan(t *testing.T) {
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	planDir := filepath.Join(plansDir, "good")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	prd := `name: Good Plan
slug: good
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A test plan
why: Testing
outcome: Tests pass
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))

	issue := `id: 1
slug: first
name: First Issue
track: intent
status: done
priority: high
points: 2
labels: []
blocked_by: []
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "001-first.yaml"), []byte(issue), 0o644))
	require.NoError(t, os.Chdir(dir))

	root := NewAgentRootCmd("test")
	root.AddCommand(NewAgentValidateCmd())
	root.SetArgs([]string{"validate", "good"})
	var out strings.Builder
	root.SetOut(&out)

	err := root.Execute()

	require.NoError(t, err)

	var result agentValidationResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &result))
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestAgentValidate_InvalidPlan_StructuredErrors(t *testing.T) {
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	planDir := filepath.Join(plansDir, "bad")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	// PRD with empty required fields
	prd := `name: ""
slug: bad
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: ""
why: ""
outcome: ""
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))

	// Issue with empty required fields
	issue := `id: 1
slug: ""
name: ""
track: intent
status: done
priority: high
points: 2
labels: []
blocked_by: []
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "001-bad.yaml"), []byte(issue), 0o644))
	require.NoError(t, os.Chdir(dir))

	root := NewAgentRootCmd("test")
	root.AddCommand(NewAgentValidateCmd())
	root.SetArgs([]string{"validate", "bad"})
	var out strings.Builder
	root.SetOut(&out)

	err := root.Execute()

	require.NoError(t, err)

	var result agentValidationResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &result))
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)

	// PRD errors should reference prd.yaml
	var prdErrors []agentValidationError
	for _, e := range result.Errors {
		if e.File == "bad/prd.yaml" {
			prdErrors = append(prdErrors, e)
		}
	}
	assert.NotEmpty(t, prdErrors, "expected PRD validation errors with file prd.yaml")
	for _, e := range prdErrors {
		assert.Equal(t, "error", e.Severity)
		assert.NotEmpty(t, e.Message)
	}

	// Issue errors should reference the issue filename
	var issueErrors []agentValidationError
	for _, e := range result.Errors {
		if strings.Contains(e.File, "issues/") {
			issueErrors = append(issueErrors, e)
		}
	}
	assert.NotEmpty(t, issueErrors, "expected issue validation errors with issue filename")
	for _, e := range issueErrors {
		assert.Equal(t, "error", e.Severity)
		assert.NotEmpty(t, e.Message)
	}
}

func TestAgentValidate_CrossRefErrors_HaveFileContext(t *testing.T) {
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	planDir := filepath.Join(plansDir, "xref")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	// PRD that tracks "intent" only
	prd := `name: Cross Ref Plan
slug: xref
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: A test plan
why: Testing
outcome: Tests pass
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))

	// Issue that references a non-existent dependency (issue #99 doesn't exist)
	issue := `id: 1
slug: orphan
name: Orphan Issue
track: intent
status: done
priority: high
points: 2
labels: []
blocked_by:
  - 99
blocking: []
created: "2025-01-01"
updated: "2025-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "001-orphan.yaml"), []byte(issue), 0o644))
	require.NoError(t, os.Chdir(dir))

	root := NewAgentRootCmd("test")
	root.AddCommand(NewAgentValidateCmd())
	root.SetArgs([]string{"validate", "xref"})
	var out strings.Builder
	root.SetOut(&out)

	err := root.Execute()

	require.NoError(t, err)

	var result agentValidationResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &result))
	assert.False(t, result.Valid)

	var crossRefErrors []agentValidationError
	for _, e := range result.Errors {
		if e.Field == "cross_ref" {
			crossRefErrors = append(crossRefErrors, e)
		}
	}
	assert.NotEmpty(t, crossRefErrors, "expected cross-ref errors")
	for _, e := range crossRefErrors {
		assert.Equal(t, "error", e.Severity)
		assert.NotEmpty(t, e.Message)
	}
}
