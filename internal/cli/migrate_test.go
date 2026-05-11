package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_ConvertsConfigAndPlanFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("plans_dir: ./.plan-bender/plans/\nmax_points: 5\n"), 0o644))

	planDir := filepath.Join(dir, ".plan-bender", "plans", "demo")
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"),
		[]byte("name: Demo\nslug: demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(issuesDir, "1-thing.yaml"),
		[]byte("id: 1\nslug: thing\n"), 0o644))

	cmd := NewMigrateCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	// Originals removed, JSON written.
	for _, removed := range []string{
		filepath.Join(dir, ".plan-bender.yaml"),
		filepath.Join(planDir, "prd.yaml"),
		filepath.Join(issuesDir, "1-thing.yaml"),
	} {
		_, err := os.Stat(removed)
		assert.True(t, os.IsNotExist(err), "expected %s removed", removed)
	}

	cfg, err := os.ReadFile(filepath.Join(dir, ".plan-bender.json"))
	require.NoError(t, err)
	assert.Contains(t, string(cfg), `"plans_dir"`)
	assert.Contains(t, string(cfg), `"max_points": 5`)

	prd, err := os.ReadFile(filepath.Join(planDir, "prd.json"))
	require.NoError(t, err)
	assert.Contains(t, string(prd), `"slug": "demo"`)
}

func TestMigrate_IsIdempotent_SkipsWhenJSONExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	// Both .yaml and .json present — migrate must skip the conversion and
	// leave .yaml intact so the user can investigate the conflict.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("max_points: 5\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"),
		[]byte(`{"max_points": 9}`), 0o644))

	cmd := NewMigrateCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	// Both files still present.
	_, err := os.Stat(filepath.Join(dir, ".plan-bender.yaml"))
	assert.NoError(t, err, "yaml must not be removed when json sibling exists")
	jsonData, err := os.ReadFile(filepath.Join(dir, ".plan-bender.json"))
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "9", "existing json must not be overwritten")
}

func TestMigrate_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("max_points: 5\n"), 0o644))

	cmd := NewMigrateCmd()
	cmd.SetArgs([]string{"--dry-run"})
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	_, err := os.Stat(filepath.Join(dir, ".plan-bender.yaml"))
	assert.NoError(t, err, "yaml must remain in dry-run mode")
	_, err = os.Stat(filepath.Join(dir, ".plan-bender.json"))
	assert.True(t, os.IsNotExist(err), "json must not be written in dry-run mode")
}
