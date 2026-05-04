package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/dispatch"
)

func setupDispatchCLI(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# r\n"), 0o644))
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender.yaml"),
		[]byte("plans_dir: ./.plan-bender/plans/\nagents:\n  claude-code: true\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".plan-bender", "plans", "demo", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender", "plans", "demo", "prd.yaml"),
		[]byte(validDemoPrd), 0o644))
	require.NoError(t, os.Chdir(root))
	return root
}

const dispatchCLIIssue = `id: 1
slug: alpha
name: Alpha
track: intent
status: %s
priority: high
points: 1
labels: [%s]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2026-04-30"
tdd: true
outcome: out
scope: scope
acceptance_criteria: ["ok"]
steps: ["x — y"]
use_cases: ["UC-1"]
`

func writeDispatchCLIIssue(t *testing.T, root, status, label string) {
	t.Helper()
	body := fmt.Sprintf(dispatchCLIIssue, status, label)
	path := filepath.Join(root, ".plan-bender", "plans", "demo", "issues", "1-alpha.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestDispatchCmd_AllDoneExitsZero(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "done", "AFK")

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	require.NoError(t, cmd.Execute())
}

func TestDispatchCmd_HITLOnlyReturnsHITLError(t *testing.T) {
	root := setupDispatchCLI(t)
	writeDispatchCLIIssue(t, root, "todo", "HITL")

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"demo"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, IsHITLOnly(err), "expected IsHITLOnly to recognize the error, got %v", err)
	assert.Contains(t, out.String(), "HITL")
}

func TestIsHITLOnly_RecognizesWrappedError(t *testing.T) {
	wrapped := fmt.Errorf("dispatch failed: %w", dispatch.ErrHITLOnly)
	assert.True(t, IsHITLOnly(wrapped))

	other := errors.New("something else")
	assert.False(t, IsHITLOnly(other))
}

func TestDispatchCmd_UnknownPlanReturnsError(t *testing.T) {
	setupDispatchCLI(t)

	cmd := NewDispatchCmd()
	cmd.SetArgs([]string{"ghost"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.False(t, IsHITLOnly(err), "unknown plan must not be confused with HITL")
}
