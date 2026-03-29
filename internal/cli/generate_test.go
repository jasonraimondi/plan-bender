package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSkills_CreatesSkillFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	cmd := NewGenerateSkillsCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "8 skills generated")

	// Verify skill dirs exist
	entries, err := os.ReadDir(filepath.Join(dir, ".plan-bender", "skills"))
	require.NoError(t, err)
	assert.Len(t, entries, 8)

	// Each has a SKILL.md with frontmatter
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", e.Name(), "SKILL.md"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(data), "---"), "SKILL.md should start with frontmatter")
	}
}

func TestGenerateSkills_UsesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Create a local override template
	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-interview-me.skill.tmpl"),
		[]byte("Custom content for {{.plans_dir}}"),
		0o644,
	))

	cmd := NewGenerateSkillsCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", "bender-interview-me", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Custom content for ./.plan-bender/plans/")
}
