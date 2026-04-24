package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runGenerateCmd(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewGenerateCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestGenerateCmd_WritesSkillsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	out := runGenerateCmd(t)

	assert.Contains(t, out, "skills generated")
	assert.Contains(t, out, "Skills:")

	// Generated skill sources exist
	entries, err := os.ReadDir(filepath.Join(dir, ".plan-bender", "skills", "claude-code"))
	require.NoError(t, err)
	assert.NotEmpty(t, entries)

	// Symlinks placed in the agent's project dir
	linkDir := filepath.Join(dir, ".claude", "skills")
	linkEntries, err := os.ReadDir(linkDir)
	require.NoError(t, err)
	require.NotEmpty(t, linkEntries)

	info, err := os.Lstat(filepath.Join(linkDir, linkEntries[0].Name()))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "skill entry should be a symlink")
}

func TestGenerateCmd_RepicksUpTemplateOverride(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Initial generation with stock templates
	runGenerateCmd(t)

	skillPath := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "SKILL.md")
	before, err := os.ReadFile(skillPath)
	require.NoError(t, err)

	// Add an override template and regenerate
	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-interview-me.skill.tmpl"),
		[]byte("OVERRIDE {{.plans_dir}}"),
		0o644,
	))

	runGenerateCmd(t)

	after, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.NotEqual(t, string(before), string(after), "regenerate should pick up the override")
	assert.Contains(t, string(after), "OVERRIDE")
}

func TestGenerateCmd_Aliases(t *testing.T) {
	cmd := NewGenerateCmd()
	assert.ElementsMatch(t, []string{"gen"}, cmd.Aliases)
}

func TestGenerateCmd_InvalidConfigReportsError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Invalid YAML forces config load failure
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("plans_dir: [not-a-string\n"),
		0o644,
	))

	cmd := NewGenerateCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SetArgs(nil)

	err := cmd.Execute()
	assert.Error(t, err)
}
