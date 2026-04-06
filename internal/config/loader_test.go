package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultsWhenNoFiles(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, Defaults(), cfg)
}

func TestLoad_ProjectYAMLMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "max_points: 5\nplans_dir: ./custom/\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.MaxPoints)
	assert.Equal(t, "./custom/", cfg.PlansDir)
}

func TestLoad_LocalOverridesProject(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "max_points: 5\n")
	writeYAML(t, filepath.Join(dir, ".plan-bender.local.yaml"), "max_points: 8\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.MaxPoints)
}

func TestLoad_MalformedYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "{{invalid yaml")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".plan-bender.yaml")
}

func TestLoad_ThreeLayerPrecedence(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, ".config", "plan-bender")
	require.NoError(t, os.MkdirAll(global, 0o755))
	writeYAML(t, filepath.Join(global, "defaults.yaml"), "max_points: 2\nplans_dir: ./global/\n")
	writeYAML(t, filepath.Join(dir, "project", ".plan-bender.yaml"), "max_points: 5\n")
	writeYAML(t, filepath.Join(dir, "project", ".plan-bender.local.yaml"), "max_points: 8\n")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "project"), 0o755))

	cfg, err := loadWithHome(filepath.Join(dir, "project"), dir)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.MaxPoints)
	assert.Equal(t, "./global/", cfg.PlansDir) // from global, not overridden
}

func TestLoad_ArraysReplaceBetweenLayers(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "tracks:\n  - alpha\n  - beta\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta"}, cfg.Tracks)
}

func TestLoad_DefaultAgents(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 1)
	assert.Equal(t, "claude-code", cfg.Agents[0].Name)
}

func TestLoad_MultipleAgentsFromYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "agents:\n  claude-code: true\n  openclaw: true\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 2)
	names := []string{cfg.Agents[0].Name, cfg.Agents[1].Name}
	assert.Contains(t, names, "claude-code")
	assert.Contains(t, names, "openclaw")
}

func TestLoad_OldAgentsArrayMigrated(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "agents:\n  - claude-code\n  - openclaw\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 2)
	names := []string{cfg.Agents[0].Name, cfg.Agents[1].Name}
	assert.Contains(t, names, "claude-code")
	assert.Contains(t, names, "openclaw")
}

func TestLoad_NewAgentsMapUntouched(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "agents:\n  claude-code: true\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 1)
	assert.Equal(t, "claude-code", cfg.Agents[0].Name)
}

func TestLoad_InstallTargetReturnsMigrationError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "install_target: project\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents:")
}

func TestLoad_InstallTargetInLocalLayerReturnsMigrationError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "max_points: 5\n")
	writeYAML(t, filepath.Join(dir, ".plan-bender.local.yaml"), "install_target: user\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
}

func TestLoad_InstallTargetWithAgentsStillFails(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "install_target: project\nagents:\n  claude-code: true\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents:")
}

func TestLoad_CleanConfigWithoutInstallTarget(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "max_points: 5\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.MaxPoints)
}

func TestLoad_ExpandsEnvVarsInLinearConfig(t *testing.T) {
	t.Setenv("PB_TEST_LINEAR_KEY", "lin_api_fromenv")
	t.Setenv("PB_TEST_LINEAR_TEAM", "ENG")

	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"),
		"linear:\n  enabled: true\n  api_key: $PB_TEST_LINEAR_KEY\n  team: $PB_TEST_LINEAR_TEAM\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "lin_api_fromenv", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestLoad_BackendLinearMigratesToLinearEnabled(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"),
		"backend: linear\nlinear:\n  api_key: sk-test\n  team: ENG\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Linear.Enabled)
	assert.Equal(t, "sk-test", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestLoad_BackendYAMLFSSilentlyDropped(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"),
		"backend: yaml-fs\nmax_points: 5\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.Linear.Enabled)
	assert.Equal(t, 5, cfg.MaxPoints)
}

func TestLoad_BackendLinearWithExistingEnabledPreserved(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"),
		"backend: linear\nlinear:\n  enabled: true\n  api_key: sk-test\n  team: ENG\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Linear.Enabled)
}

func TestLoad_NoBackendKeyLoadsNormally(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"),
		"max_points: 7\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.Linear.Enabled)
	assert.Equal(t, 7, cfg.MaxPoints)
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
