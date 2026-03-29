package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstall_GeneratesAndCreatesSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	installCmd := NewInstallCmd()
	var out strings.Builder
	installCmd.SetOut(&out)
	require.NoError(t, installCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "8 skills generated")
	assert.Contains(t, output, "8 skills installed")

	// Verify symlinks exist
	targetDir := filepath.Join(dir, ".claude", "skills")
	entries, err := os.ReadDir(targetDir)
	require.NoError(t, err)
	assert.Len(t, entries, 8)

	// Each should be a symlink
	for _, e := range entries {
		info, err := os.Lstat(filepath.Join(targetDir, e.Name()))
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0, "%s should be a symlink", e.Name())
	}
}

func TestInstall_UpdatesGitignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	installCmd := NewInstallCmd()
	installCmd.SetOut(&strings.Builder{})
	require.NoError(t, installCmd.Execute())

	// Check .gitignore
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, ".plan-bender/")
	assert.Contains(t, content, ".claude/skills/bender-*")
	assert.Contains(t, content, ".plan-bender.local.yaml")
}

func TestInstall_ReplacesExistingSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Install twice — should not error
	for i := 0; i < 2; i++ {
		installCmd := NewInstallCmd()
		installCmd.SetOut(&strings.Builder{})
		require.NoError(t, installCmd.Execute())
	}
}
