package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_JSON(t *testing.T) {
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
description: A test
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

	cmd := NewValidateCmd()
	cmd.SetArgs([]string{"test-plan", "--json"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	var result schema.PlanValidationResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &result))

	assert.True(t, result.Valid)
	assert.Empty(t, result.PRD.Errors)
	assert.Empty(t, result.CrossRef)
	assert.Empty(t, result.Cycles)
}

func TestValidate_JSON_WithErrors(t *testing.T) {
	dir := t.TempDir()

	plansDir := filepath.Join(dir, ".plan-bender", "plans")
	planDir := filepath.Join(plansDir, "bad-plan")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	// PRD missing required fields
	prd := `name: ""
slug: bad-plan
status: active
created: "2025-01-01"
updated: "2025-01-02"
description: ""
why: ""
outcome: ""
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))

	issue := `id: 1
slug: first
name: First
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

	cmd := NewValidateCmd()
	cmd.SetArgs([]string{"bad-plan", "--json"})
	var out strings.Builder
	cmd.SetOut(&out)
	// --json still returns nil even on validation failure (JSON encodes the result)
	require.NoError(t, cmd.Execute())

	var result schema.PlanValidationResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &result))

	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.PRD.Errors)
}
