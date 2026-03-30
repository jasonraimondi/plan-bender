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
	assert.Equal(t, BackendYAMLFS, cfg.Backend) // default preserved
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
	assert.Equal(t, []string{"claude-code"}, cfg.Agents)
}

func TestLoad_MultipleAgentsFromYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "agents:\n  - claude-code\n  - openclaw\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code", "openclaw"}, cfg.Agents)
}

func TestLoad_InstallTargetReturnsMigrationError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "install_target: project\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents: [claude-code]")
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
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "install_target: project\nagents:\n  - claude-code\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents: [claude-code]")
}

func TestLoad_CleanConfigWithoutInstallTarget(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, ".plan-bender.yaml"), "max_points: 5\n")

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.MaxPoints)
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
