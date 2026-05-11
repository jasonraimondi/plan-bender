package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigCheck_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"), []byte("{}"), 0o644))

	r := configCheck(dir)
	assert.True(t, r.Pass)
	assert.Equal(t, "config", r.Name)
	assert.Equal(t, "valid", r.Message)
}

func TestConfigCheck_MissingConfig(t *testing.T) {
	dir := t.TempDir()

	r := configCheck(dir)
	assert.True(t, r.Pass)
}

func TestConfigCheck_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".plan-bender.json"),
		[]byte(`{"max_points": -1}`),
		0o644,
	))

	r := configCheck(dir)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "max_points")
}

func TestSkillsCheck_NoSkillsGenerated(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	r := skillsCheck(dir, cfg)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "skills not generated")
}

func TestSkillsCheck_Generated_NoSymlinks(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	skillDir := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	r := skillsCheck(dir, cfg)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "symlink missing")
}

func TestSkillsCheck_FullySetUp(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	skillDir := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	targetDir := filepath.Join(dir, ".claude", "skills")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.Symlink(skillDir, filepath.Join(targetDir, "bender-test-skill")))

	r := skillsCheck(dir, cfg)
	assert.True(t, r.Pass)
	assert.Contains(t, r.Message, "1 skills")
	assert.Contains(t, r.Message, "1 symlinks")
}

func TestPlansDirCheck_Exists(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	plansDir := filepath.Join(dir, cfg.PlansDir)
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	r := plansDirCheck(dir, cfg)
	assert.True(t, r.Pass)
	assert.Equal(t, "plans dir", r.Name)
}

func TestPlansDirCheck_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	r := plansDirCheck(dir, cfg)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "does not exist")
}

func TestVersionCheck_NotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	r := versionCheck("1.0.0")
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "plan-bender-agent not found")
	assert.Contains(t, r.Message, "install plan-bender-agent or check PATH")
}

func TestVersionCheck_Match(t *testing.T) {
	binDir := t.TempDir()
	script := filepath.Join(binDir, "plan-bender-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"plan-bender-agent version 1.2.3\"\n"), 0o755))
	t.Setenv("PATH", binDir)

	r := versionCheck("1.2.3")
	assert.True(t, r.Pass)
	assert.Equal(t, "1.2.3", r.Message)
}

func TestVersionCheck_Mismatch(t *testing.T) {
	binDir := t.TempDir()
	script := filepath.Join(binDir, "plan-bender-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"plan-bender-agent version 0.9.0\"\n"), 0o755))
	t.Setenv("PATH", binDir)

	r := versionCheck("1.2.3")
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, "mismatch")
	assert.Contains(t, r.Message, "pb=1.2.3")
	assert.Contains(t, r.Message, "pba=0.9.0")
}

func TestLinearCheck_Disabled(t *testing.T) {
	cfg := config.Defaults()

	r := linearCheck(cfg)
	assert.True(t, r.Pass)
	assert.Contains(t, r.Message, "skipped")
	assert.Contains(t, r.Message, "not enabled")
}

func TestGitignoreCheck_Managed(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.ManageGitignore = true

	r := gitignoreCheck(dir, cfg)
	assert.True(t, r.Pass)
	assert.Equal(t, "gitignore", r.Name)
	assert.Equal(t, "managed", r.Message)
}

func TestGitignoreCheck_UnmanagedLocalIgnored(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".gitignore"),
		[]byte("node_modules/\n.plan-bender.local.json\n"),
		0o644,
	))

	r := gitignoreCheck(dir, cfg)
	assert.True(t, r.Pass)
	assert.Contains(t, r.Message, "unmanaged")
	assert.Contains(t, r.Message, ".plan-bender.local.json ok")
}

func TestGitignoreCheck_UnmanagedLocalMissing(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".gitignore"),
		[]byte("node_modules/\n"),
		0o644,
	))

	r := gitignoreCheck(dir, cfg)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, ".plan-bender.local.json not gitignored")
}

func TestGitignoreCheck_UnmanagedNoGitignoreFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	r := gitignoreCheck(dir, cfg)
	assert.False(t, r.Pass)
	assert.Contains(t, r.Message, ".plan-bender.local.json not gitignored")
}

func TestRunChecks_ReturnsAllChecks(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Defaults()

	t.Setenv("PATH", t.TempDir())
	results := RunChecks(dir, cfg, "dev")
	assert.Len(t, results, 6)

	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Name
	}
	assert.Contains(t, names, "config")
	assert.Contains(t, names, "skills")
	assert.Contains(t, names, "plans dir")
	assert.Contains(t, names, "versions")
	assert.Contains(t, names, "linear")
	assert.Contains(t, names, "gitignore")
}

func TestDoctorCmd_HealthySetup(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"), []byte(`{"manage_gitignore": true}`), 0o644))

	setupCmd := NewSetupCmd("test")
	setupCmd.SetOut(&strings.Builder{})
	require.NoError(t, setupCmd.Execute())

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	plansDir := filepath.Join(dir, cfg.PlansDir)
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	binDir := t.TempDir()
	script := filepath.Join(binDir, "plan-bender-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"plan-bender-agent version test-ver\"\n"), 0o755))
	t.Setenv("PATH", binDir)

	results := RunChecks(dir, cfg, "test-ver")
	for _, r := range results {
		assert.True(t, r.Pass, "check %q should pass: %s", r.Name, r.Message)
	}
}

func TestDoctorCmd_PrintsOutput(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"), []byte("{}"), 0o644))

	binDir := t.TempDir()
	script := filepath.Join(binDir, "plan-bender-agent")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"plan-bender-agent version dev\"\n"), 0o755))
	t.Setenv("PATH", binDir)

	cmd := NewDoctorCmd("dev")
	var out strings.Builder
	cmd.SetOut(&out)

	cfg, _ := config.Load(dir)
	results := RunChecks(dir, cfg, "dev")

	configResult := results[0]
	assert.True(t, configResult.Pass)

	linearResult := results[4]
	assert.True(t, linearResult.Pass)
	assert.Contains(t, linearResult.Message, "skipped")
}
