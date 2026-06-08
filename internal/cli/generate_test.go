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
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	var out strings.Builder
	count, err := GenerateSkills(dir, cfg, &out)
	require.NoError(t, err)

	assert.Equal(t, 8, count)
	assert.Contains(t, out.String(), "8 skills generated")

	agentDir := filepath.Join(dir, ".plan-bender", "skills", "claude-code")
	entries, err := os.ReadDir(agentDir)
	require.NoError(t, err)
	assert.Len(t, entries, 8)

	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(agentDir, e.Name(), "SKILL.md"))
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(data), "---"), "SKILL.md should start with frontmatter")
	}
}

func TestGenerateSkills_OmitsBackendSkillWhenLinearDisabled(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	require.False(t, cfg.Linear.Enabled, "default config has Linear disabled")

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-sync-linear"))
	assert.True(t, os.IsNotExist(err), "bender-sync-linear must not be generated when Linear is disabled")
}

func TestGenerateSkills_EmitsBackendSkillWhenLinearEnabled(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	cfg.Linear.Enabled = true

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-sync-linear", "SKILL.md"))
	require.NoError(t, err, "bender-sync-linear must be generated when Linear is enabled")
}

func TestGenerateSkills_OmitsImplementSkillsWhenNoImplement(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	cfg.NoImplement = true

	var out strings.Builder
	count, err := GenerateSkills(dir, cfg, &out)
	require.NoError(t, err)

	// Default config generates 8 skills; no_implement drops the 2 implement ones.
	assert.Equal(t, 6, count)

	for _, name := range []string{"bender-implement-prd", "bender-implement-issue"} {
		_, err := os.Stat(filepath.Join(dir, ".plan-bender", "skills", "claude-code", name))
		assert.True(t, os.IsNotExist(err), "%s must not be generated when no_implement is set", name)
	}

	_, err = os.Stat(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-write-plan", "SKILL.md"))
	require.NoError(t, err, "non-implement skills still generated")
}

func TestGenerateSkills_RemovesImplementSkillsWhenFlagFlipped(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	implPath := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-implement-prd")
	_, err = os.Stat(implPath)
	require.NoError(t, err, "implement skill present before flag flip")

	cfg.NoImplement = true
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(implPath)
	assert.True(t, os.IsNotExist(err), "implement skill removed on regenerate after no_implement turned on")
}

func TestGenerateSkills_UsesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	overrideDir := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "SKILL.md.tmpl"),
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
	chdir(t, dir)

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

	for _, agent := range []string{"claude-code", "openclaw"} {
		entries, err := os.ReadDir(filepath.Join(dir, ".plan-bender", "skills", agent))
		require.NoError(t, err)
		assert.Len(t, entries, 8, "agent %s should have 8 skill dirs", agent)
	}
}

func TestGenerateCmd_NoForkedNextTemplates_NoWarning(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

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
	chdir(t, dir)

	fork := filepath.Join(dir, ".plan-bender", "templates", "bender-implement-prd")
	require.NoError(t, os.MkdirAll(fork, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-orchestrator")
}

func TestGenerateCmd_ForkedOrchestrator_Warns(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	fork := filepath.Join(dir, ".plan-bender", "templates", "bender-orchestrator")
	require.NoError(t, os.MkdirAll(fork, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-orchestrator")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-implement-prd")
}

func TestGenerateCmd_BothForks_WarnsBoth(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	for _, name := range []string{"bender-implement-prd", "bender-orchestrator"} {
		fork := filepath.Join(dir, ".plan-bender", "templates", name)
		require.NoError(t, os.MkdirAll(fork, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))
	}

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "bender-orchestrator")
	assert.Equal(t, 2, strings.Count(out, "warning:"), "expected exactly two warning lines")
}

func TestGenerateSkills_EmitsSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Inject supporting files onto an existing skill via override: one verbatim
	// file and one nested .tmpl file.
	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "refs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CONTEXT-FORMAT.md"), []byte("verbatim {{.plans_dir}}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "refs", "notes.md.tmpl"), []byte("rendered {{.plans_dir}}"), 0o644))

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	out := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me")

	verbatim, err := os.ReadFile(filepath.Join(out, "CONTEXT-FORMAT.md"))
	require.NoError(t, err)
	assert.Equal(t, "verbatim {{.plans_dir}}", string(verbatim), "non-.tmpl file copied byte-for-byte")

	nested, err := os.ReadFile(filepath.Join(out, "refs", "notes.md"))
	require.NoError(t, err)
	assert.Equal(t, "rendered ./.plan-bender/plans/", string(nested), ".tmpl rendered, suffix stripped, subdir preserved")

	_, err = os.Stat(filepath.Join(out, "SKILL.md"))
	require.NoError(t, err, "body still emitted")
}

func TestGenerateSkills_RemovesStaleSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "EXTRA.md"), []byte("x"), 0o644))

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	staleOut := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "EXTRA.md")
	_, err = os.Stat(staleOut)
	require.NoError(t, err, "supporting file present after first generate")

	require.NoError(t, os.Remove(filepath.Join(base, "EXTRA.md")))
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(staleOut)
	assert.True(t, os.IsNotExist(err), "stale supporting file removed on regenerate")
}

func TestGenerateSkills_RemovesStaleSkillDir(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	// Simulate a skill an older pb version generated that is no longer in the
	// template set (renamed or removed).
	staleDir := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-OLD-removed")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("stale"), 0o644))

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(staleDir)
	assert.True(t, os.IsNotExist(err), "stale skill dir must be removed on regenerate")

	_, err = os.Stat(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-write-plan"))
	require.NoError(t, err, "current skill still generated")
}

func TestGenerateCmd_FlatOverride_WarnsMigration(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-interview-me.skill.tmpl"),
		[]byte("stale flat override"),
		0o644,
	))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-interview-me.skill.tmpl")
	assert.Contains(t, out, "no longer read")
	assert.Contains(t, out, "bender-interview-me/SKILL.md.tmpl")
	assert.NotContains(t, out, "pba next")
}
