package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validPRD with no issues lets us isolate parse failures to the issue file.
const malformedTestPRD = `name: Bad
slug: bad
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: a plan
why: testing
outcome: ok
`

// malformedIssue triggers the colon-in-list footgun: a list item with `: `
// mid-prose decodes as a !!map, not a string. yaml.v3 reports it on the
// affected list-item line.
const malformedTestIssue = `id: 1
slug: bad
name: Bad Issue
track: intent
status: todo
priority: high
points: 1
labels: []
blocked_by: []
blocking: []
created: "2026-01-01"
updated: "2026-01-02"
outcome: done
scope: small
acceptance_criteria: []
steps:
  - first step
  - some/path/file.ts — implement methodName: when X is true do Y
use_cases: []
`

func setupMalformedPlan(t *testing.T, slug string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	planDir := filepath.Join(dir, ".plan-bender", "plans", slug)
	require.NoError(t, os.MkdirAll(filepath.Join(planDir, "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(malformedTestPRD), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "issues", "1-bad.yaml"), []byte(malformedTestIssue), 0o644))
	return dir
}

func TestStatus_MalformedYAML_ReturnsInvalidPlanWithFileAndLine(t *testing.T) {
	setupMalformedPlan(t, "bad")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"status", "bad"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)
	require.Error(t, err)

	var resp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &resp))
	assert.Equal(t, string(ErrInvalidPlan), resp.Code, "raw output: %s", out.String())
	assert.Contains(t, resp.File, "issues/1-bad.yaml")
	assert.Greater(t, resp.Line, 0, "line number should be parsed from yaml error")
	assert.NotEmpty(t, resp.Hint)
}

func TestNext_MalformedYAML_ReturnsInvalidPlan(t *testing.T) {
	setupMalformedPlan(t, "bad")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"next", "bad"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)
	require.Error(t, err)

	var resp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &resp))
	assert.Equal(t, string(ErrInvalidPlan), resp.Code)
}

func TestContext_MalformedYAML_ReturnsInvalidPlan(t *testing.T) {
	setupMalformedPlan(t, "bad")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"context", "bad"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)
	require.Error(t, err)

	var resp errorJSON
	require.NoError(t, json.Unmarshal([]byte(out.String()), &resp))
	assert.Equal(t, string(ErrInvalidPlan), resp.Code)
}

func TestOpenErrorToAgent_PlanNotFoundStillBeatsParseCheck(t *testing.T) {
	// fs.ErrNotExist must short-circuit before we look for ParseError, since
	// a missing plan dir surfaces as an ENOENT-wrapped error.
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"status", "ghost"})
	var out strings.Builder
	root.SetOut(&out)

	err := ExecuteAgent(root)
	require.Error(t, err)

	var agentErr *AgentError
	require.True(t, errors.As(err, &agentErr))
	assert.Equal(t, ErrPlanNotFound, agentErr.Code)
}
