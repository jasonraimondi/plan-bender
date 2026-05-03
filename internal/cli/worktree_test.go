package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

const worktreeIssueYAML = `id: 7
slug: middleware
name: Middleware
track: intent
status: todo
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2026-04-30"
tdd: true
outcome: It works
scope: Small
acceptance_criteria: ["It works"]
steps: ["Target — works"]
use_cases: ["UC-1"]
`

func setupWorktreeRepo(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	plansDir := filepath.Join(root, ".plan-bender", "plans", "auth", "issues")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "../prd.yaml"), []byte(validAuthPrd), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "7-middleware.yaml"), []byte(worktreeIssueYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".plan-bender.yaml"), []byte("plans_dir: ./.plan-bender/plans/\nagents:\n  claude-code: true\n"), 0o644))

	require.NoError(t, os.Chdir(root))
	return root
}

func TestWorktreeCreate_AgentModeJSON(t *testing.T) {
	root := setupWorktreeRepo(t)

	agentCmd := NewAgentRootCmd("test")
	agentCmd.SetArgs([]string{"worktree", "create", "auth", "7"})
	var out strings.Builder
	agentCmd.SetOut(&out)
	require.NoError(t, agentCmd.Execute())

	var got map[string]string
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	assert.Equal(t, "tester/auth--7-middleware", got["branch"])
	assert.Contains(t, got["path"], "repo-wt/auth/7-middleware")
	assert.Equal(t, "in-progress", got["status"])

	_, err := os.Stat(got["path"])
	assert.NoError(t, err)

	iss := loadWorktreeIssue(t, root)
	assert.Equal(t, "in-progress", iss.Status, "issue YAML status must be in-progress after create")
	require.NotNil(t, iss.Branch)
	assert.Equal(t, "tester/auth--7-middleware", *iss.Branch, "issue YAML branch must be set after create")
}

func TestWorktreeCreate_HumanMode(t *testing.T) {
	root := setupWorktreeRepo(t)

	cmd := NewWorktreeCmd()
	cmd.SetArgs([]string{"create", "auth", "7"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "branch:")
	assert.Contains(t, out.String(), "tester/auth--7-middleware")
	assert.Contains(t, out.String(), "status:")
	assert.Contains(t, out.String(), "in-progress")

	iss := loadWorktreeIssue(t, root)
	assert.Equal(t, "in-progress", iss.Status)
	require.NotNil(t, iss.Branch)
	assert.Equal(t, "tester/auth--7-middleware", *iss.Branch)
}

func loadWorktreeIssue(t *testing.T, root string) schema.IssueYaml {
	t.Helper()
	path := filepath.Join(root, ".plan-bender", "plans", "auth", "issues", "7-middleware.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var iss schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &iss))
	return iss
}

func TestWorktreeCreate_UnknownIssue(t *testing.T) {
	setupWorktreeRepo(t)

	cmd := NewWorktreeCmd()
	cmd.SetArgs([]string{"create", "auth", "999"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)
}

func TestWorktreeGC_AgentModeReturnsRemoved(t *testing.T) {
	root := setupWorktreeRepo(t)

	createCmd := NewWorktreeCmd()
	createCmd.SetArgs([]string{"create", "auth", "7"})
	var out strings.Builder
	createCmd.SetOut(&out)
	require.NoError(t, createCmd.Execute())

	require.NoError(t, os.Chdir(root))

	gcRoot := NewAgentRootCmd("test")
	gcRoot.SetArgs([]string{"worktree", "gc", "auth"})
	var gcOut strings.Builder
	gcRoot.SetOut(&gcOut)
	require.NoError(t, gcRoot.Execute())

	var got map[string][]string
	require.NoError(t, json.Unmarshal([]byte(gcOut.String()), &got))
	require.Len(t, got["removed"], 1)
	assert.Contains(t, got["removed"][0], "7-middleware")
}

func TestWorktreeGC_NoMatchesIsClean(t *testing.T) {
	setupWorktreeRepo(t)

	cmd := NewWorktreeCmd()
	cmd.SetArgs([]string{"gc", "auth"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "no worktrees")
}
