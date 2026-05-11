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

func TestLoad_ProjectJSONMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"max_points": 5, "plans_dir": "./custom/"}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.MaxPoints)
	assert.Equal(t, "./custom/", cfg.PlansDir)
}

func TestLoad_LocalOverridesProject(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"max_points": 5}`)
	writeJSON(t, filepath.Join(dir, ".plan-bender.local.json"), `{"max_points": 8}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.MaxPoints)
}

func TestLoad_MalformedJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), "{not valid json")

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".plan-bender.json")
}

func TestLoad_ThreeLayerPrecedence(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, ".config", "plan-bender")
	require.NoError(t, os.MkdirAll(global, 0o755))
	writeJSON(t, filepath.Join(global, "defaults.json"), `{"max_points": 2, "plans_dir": "./global/"}`)
	writeJSON(t, filepath.Join(dir, "project", ".plan-bender.json"), `{"max_points": 5}`)
	writeJSON(t, filepath.Join(dir, "project", ".plan-bender.local.json"), `{"max_points": 8}`)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "project"), 0o755))

	cfg, err := loadWithHome(filepath.Join(dir, "project"), dir)
	require.NoError(t, err)
	assert.Equal(t, 8, cfg.MaxPoints)
	assert.Equal(t, "./global/", cfg.PlansDir) // from global, not overridden
}

func TestLoad_ArraysReplaceBetweenLayers(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"tracks": ["alpha", "beta"]}`)

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

func TestLoad_MultipleAgentsFromJSON(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"agents": {"claude-code": true, "openclaw": true}}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 2)
	names := []string{cfg.Agents[0].Name, cfg.Agents[1].Name}
	assert.Contains(t, names, "claude-code")
	assert.Contains(t, names, "openclaw")
}

func TestLoad_OldAgentsArrayMigrated(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"agents": ["claude-code", "openclaw"]}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 2)
	names := []string{cfg.Agents[0].Name, cfg.Agents[1].Name}
	assert.Contains(t, names, "claude-code")
	assert.Contains(t, names, "openclaw")
}

func TestLoad_OldReviewWithUserArrayMigrated(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"review_with_user": []}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.ReviewWithUser)
}

func TestLoad_OldReviewWithUserNonEmptyArrayMigratedToTrue(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"review_with_user": ["prd"]}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.ReviewWithUser)
}

func TestLoad_NewAgentsMapUntouched(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"agents": {"claude-code": true}}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 1)
	assert.Equal(t, "claude-code", cfg.Agents[0].Name)
}

func TestLoad_InstallTargetReturnsMigrationError(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"install_target": "project"}`)

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents:")
}

func TestLoad_InstallTargetInLocalLayerReturnsMigrationError(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"max_points": 5}`)
	writeJSON(t, filepath.Join(dir, ".plan-bender.local.json"), `{"install_target": "user"}`)

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
}

func TestLoad_InstallTargetWithAgentsStillFails(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"install_target": "project", "agents": {"claude-code": true}}`)

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install_target")
	assert.Contains(t, err.Error(), "agents:")
}

func TestLoad_CleanConfigWithoutInstallTarget(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"), `{"max_points": 5}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.MaxPoints)
}

func TestLoad_ExpandsEnvVarsInLinearConfig(t *testing.T) {
	t.Setenv("PB_TEST_LINEAR_KEY", "lin_api_fromenv")
	t.Setenv("PB_TEST_LINEAR_TEAM", "ENG")

	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"),
		`{"linear": {"enabled": true, "api_key": "$PB_TEST_LINEAR_KEY", "team": "$PB_TEST_LINEAR_TEAM"}}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "lin_api_fromenv", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestLoad_BackendLinearMigratesToLinearEnabled(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"),
		`{"backend": "linear", "linear": {"api_key": "sk-test", "team": "ENG"}}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Linear.Enabled)
	assert.Equal(t, "sk-test", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestLoad_BackendYAMLFSSilentlyDropped(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"),
		`{"backend": "yaml-fs", "max_points": 5}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.Linear.Enabled)
	assert.Equal(t, 5, cfg.MaxPoints)
}

func TestLoad_BackendLinearWithExistingEnabledPreserved(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"),
		`{"backend": "linear", "linear": {"enabled": true, "api_key": "sk-test", "team": "ENG"}}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Linear.Enabled)
}

func TestLoad_NoBackendKeyLoadsNormally(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, ".plan-bender.json"),
		`{"max_points": 7}`)

	cfg, err := Load(dir)
	require.NoError(t, err)
	assert.False(t, cfg.Linear.Enabled)
	assert.Equal(t, 7, cfg.MaxPoints)
}

func writeJSON(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
