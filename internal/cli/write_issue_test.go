package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validIssueYAML = `id: 1
slug: do-the-thing
name: Do the thing
track: intent
status: backlog
priority: medium
points: 2
labels: []
blocked_by: []
blocking: []
created: "2026-03-26"
updated: "2026-03-26"
tdd: false
outcome: Something works
scope: Small change
acceptance_criteria:
  - It works
steps:
  - "Target — does the thing"
use_cases:
  - User does the thing
`

func TestWriteIssue_ValidIssue(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	inputFile := filepath.Join(dir, "issue.yaml")
	require.NoError(t, os.WriteFile(inputFile, []byte(validIssueYAML), 0o644))

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan", inputFile})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.yaml"))
	assert.NoError(t, err)
}

func TestWriteIssue_StdinPipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".plan-bender", "plans"), 0o755))

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan"})
	cmd.SetIn(strings.NewReader(validIssueYAML))
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.yaml"))
	assert.NoError(t, err)
}
