package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSkills_CreatesSkillFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	var out strings.Builder
	count, err := GenerateSkills(dir, cfg, &out)
	require.NoError(t, err)

	assert.Equal(t, 8, count)
	assert.Contains(t, out.String(), "8 skills generated")

	// Verify agent subdir exists with skill dirs
	agentDir := filepath.Join(dir, ".plan-bender", "skills", "claude-code")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	assert.Len(t, entries, 8)

	// Each has a SKILL.md with frontmatter
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(agentDir, e.Name(), "SKILL.md"))
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

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Custom content for ./.plan-bender/plans/")
}

func TestGenerateSkills_MultipleAgents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	cfg.Agents = []config.ResolvedAgent{
		{Name: "claude-code"},
		{Name: "openclaw"},
	}

	var out strings.Builder
	count, err := GenerateSkills(dir, cfg, &out)
	require.NoError(t, err)

	assert.Equal(t, 16, count)
	assert.Contains(t, out.String(), "16 skills generated")

	// Both agent directories exist with skill subdirs
	for _, agent := range []string{"claude-code", "openclaw"} {
		entries, err := os.ReadDir(filepath.Join(dir, ".plan-bender", "skills", agent))
		require.NoError(t, err)
		assert.Len(t, entries, 8, "agent %s should have 8 skill dirs", agent)
	}

	// claude-code interview contains AskUserQuestionTool
	ccData, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(ccData), "AskUserQuestionTool")

	// openclaw interview uses conversational phrasing
	ocData, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", "openclaw", "bender-interview-me", "SKILL.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(ocData), "AskUserQuestionTool")
	assert.Contains(t, string(ocData), "Ask the user directly in conversation")
}

func TestGenerateCmd_NoForkedNextTemplates_NoWarning(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	assert.NotContains(t, stderr.String(), "warning:")
}

func TestGenerateCmd_ForkedImplementPrd_Warns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-implement-prd.skill.tmpl"),
		[]byte("---\nname: x\n---\nbody"),
		0o644,
	))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd.skill.tmpl")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-orchestrator.skill.tmpl")
}

func TestGenerateCmd_ForkedOrchestrator_Warns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-orchestrator.skill.tmpl"),
		[]byte("---\nname: x\n---\nbody"),
		0o644,
	))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-orchestrator.skill.tmpl")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-implement-prd.skill.tmpl")
}

func TestGenerateCmd_BothForks_WarnsBoth(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	for _, name := range []string{"bender-implement-prd.skill.tmpl", "bender-orchestrator.skill.tmpl"} {
		require.NoError(t, os.WriteFile(
			filepath.Join(overrideDir, name),
			[]byte("---\nname: x\n---\nbody"),
			0o644,
		))
	}

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd.skill.tmpl")
	assert.Contains(t, out, "bender-orchestrator.skill.tmpl")
	assert.Equal(t, 2, strings.Count(out, "warning:"), "expected exactly two warning lines")
}
