package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup_ExistingConfigSkipsFormAndInstalls(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Write a minimal config so the form is skipped
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("{}"),
		0o644,
	))

	cmd := NewSetupCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	output := out.String()
	assert.Contains(t, output, "8 skills generated")
	assert.Contains(t, output, "8 skills installed")
	assert.NotContains(t, output, "wrote") // config already existed, no write
}

func TestSetup_SymlinksToAgentProjectDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("agents:\n  - claude-code\n"),
		0o644,
	))

	cmd := NewSetupCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

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

func TestSetup_IdempotentRerunsReplaceSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("{}"),
		0o644,
	))

	for i := 0; i < 2; i++ {
		cmd := NewSetupCmd()
		cmd.SetOut(&strings.Builder{})
		require.NoError(t, cmd.Execute(), "run %d should not error", i+1)
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

	cmd := NewSetupCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".plan-bender/")
	assert.Contains(t, content, ".plan-bender.local.yaml")
	assert.Contains(t, content, ".claude/skills/bender-*") // from claude-code agent
}

func TestEnsureGitignoreForAgents_SkipsUserOnlyAgents(t *testing.T) {
	dir := t.TempDir()

	// Call with no agents — should still write base entries
	ensureGitignoreForAgents(dir, nil)

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".plan-bender/")
	assert.Contains(t, content, ".plan-bender.local.yaml")
	assert.NotContains(t, content, "bender-*") // no agents, no agent patterns
}
