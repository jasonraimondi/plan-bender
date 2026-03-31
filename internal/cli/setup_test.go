package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockValidator struct {
	err error
}

func (m *mockValidator) ListWorkflowStates(_ context.Context, _ string) (map[string]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return map[string]string{"Todo": "id-1", "Done": "id-2"}, nil
}

func testSetupCmd(deps setupDeps) *setupTestHarness {
	cmd := newSetupCmd(deps)
	var out strings.Builder
	cmd.SetOut(&out)
	return &setupTestHarness{cmd: cmd, out: &out, deps: deps}
}

type setupTestHarness struct {
	cmd  *cobra.Command
	out  *strings.Builder
	deps setupDeps
}

func (h *setupTestHarness) execute(args ...string) error {
	h.cmd.SetArgs(args)
	return h.cmd.Execute()
}

func (h *setupTestHarness) output() string {
	return h.out.String()
}

func TestSetup_FirstRunWritesDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	h := testSetupCmd(setupDeps{})
	require.NoError(t, h.execute())

	// Config file created
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "plans_dir")
	assert.Contains(t, string(data), "max_points: 3")

	output := h.output()
	assert.Contains(t, output, "Config:  .plan-bender.yaml (created)")
	assert.Contains(t, output, "Linear:  disabled")
	assert.Contains(t, output, "Ready!")
}

func TestSetup_ExistingConfigSkipsWrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("{}"),
		0o644,
	))

	h := testSetupCmd(setupDeps{})
	require.NoError(t, h.execute())

	output := h.output()
	assert.Contains(t, output, "Config:  .plan-bender.yaml (exists)")
	assert.NotContains(t, output, "(created)")
}

func TestSetup_YesFlagExitsZero(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	h := testSetupCmd(setupDeps{})
	require.NoError(t, h.execute("--yes"))
}

func TestSetup_LinearWithEnvVars(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	t.Setenv("LINEAR_API_KEY", "lin_test_key")
	t.Setenv("LINEAR_TEAM", "ENG")

	h := testSetupCmd(setupDeps{
		newValidator: func(apiKey string) linearValidator {
			assert.Equal(t, "lin_test_key", apiKey)
			return &mockValidator{}
		},
	})
	require.NoError(t, h.execute("--linear"))

	// Project config has linear.enabled: true
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender.yaml"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "enabled: true")

	// Local config has credentials
	localData, err := os.ReadFile(filepath.Join(dir, ".plan-bender.local.yaml"))
	require.NoError(t, err)
	localContent := string(localData)
	assert.Contains(t, localContent, "api_key: lin_test_key")
	assert.Contains(t, localContent, "team: ENG")

	// Credentials NOT in project config
	assert.NotContains(t, content, "lin_test_key")
	assert.NotContains(t, content, "api_key")

	output := h.output()
	assert.Contains(t, output, "Linear:  enabled")
}

func TestSetup_LinearWithInvalidCreds(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	t.Setenv("LINEAR_API_KEY", "bad_key")
	t.Setenv("LINEAR_TEAM", "BAD")

	h := testSetupCmd(setupDeps{
		newValidator: func(_ string) linearValidator {
			return &mockValidator{err: fmt.Errorf("unauthorized")}
		},
	})
	err := h.execute("--linear")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential validation failed")

	// Nothing written
	_, err = os.Stat(filepath.Join(dir, ".plan-bender.local.yaml"))
	assert.True(t, os.IsNotExist(err))
}

func TestSetup_LinearYesWithoutEnvVarsErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	t.Setenv("LINEAR_API_KEY", "")
	t.Setenv("LINEAR_TEAM", "")

	h := testSetupCmd(setupDeps{})
	err := h.execute("--yes", "--linear")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-interactive mode")
}

func TestSetup_InitAlias(t *testing.T) {
	cmd := NewSetupCmd()
	assert.Contains(t, cmd.Aliases, "init")
}

func TestSetup_RerunRegeneratesSkills(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("{}"),
		0o644,
	))

	// Run twice — should succeed both times
	for i := 0; i < 2; i++ {
		h := testSetupCmd(setupDeps{})
		require.NoError(t, h.execute(), "run %d should not error", i+1)
	}
}

func TestSetup_SymlinksToAgentProjectDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("agents:\n  - claude-code\n"),
		0o644,
	))

	h := testSetupCmd(setupDeps{})
	require.NoError(t, h.execute())

	targetDir := filepath.Join(dir, ".claude", "skills")
	entries, err := os.ReadDir(targetDir)
	require.NoError(t, err)
	assert.Len(t, entries, 8)

	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(targetDir, e.Name()))
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0, "%s should be a symlink", e.Name())
	}
}

func TestSetup_GitignoreRegistryDriven(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("agents:\n  - claude-code\n"),
		0o644,
	))

	h := testSetupCmd(setupDeps{})
	require.NoError(t, h.execute())

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".plan-bender/")
	assert.Contains(t, content, ".plan-bender.local.yaml")
	assert.Contains(t, content, ".claude/skills/bender-*")
}

func TestEnsureGitignoreForAgents_SkipsUserOnlyAgents(t *testing.T) {
	dir := t.TempDir()

	ensureGitignoreForAgents(dir, nil)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".plan-bender/")
	assert.Contains(t, content, ".plan-bender.local.yaml")
	assert.NotContains(t, content, "bender-*")
}

func TestMergeYAMLFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	err := mergeYAMLFile(path, map[string]any{
		"linear": map[string]any{"enabled": true},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "enabled: true")
}

func TestMergeYAMLFile_PreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	require.NoError(t, os.WriteFile(path, []byte("max_points: 5\nlinear:\n  team: OLD\n"), 0o644))

	err := mergeYAMLFile(path, map[string]any{
		"linear": map[string]any{"enabled": true},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "max_points: 5")
	assert.Contains(t, content, "enabled: true")
	assert.Contains(t, content, "team: OLD")
}
