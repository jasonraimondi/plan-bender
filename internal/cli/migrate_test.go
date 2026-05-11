package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
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
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	_, err := os.Stat(filepath.Join(dir, ".plan-bender.yaml"))
	assert.NoError(t, err, "yaml must not be removed when json sibling exists")
	jsonData, err := os.ReadFile(filepath.Join(dir, ".plan-bender.json"))
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "9", "existing json must not be overwritten")

	// Conflict must be surfaced — both in per-file output and summary.
	assert.Contains(t, out.String(), "conflict")
	assert.Contains(t, out.String(), "1 conflict")
}

// TestMigrate_FallsBackOnConfigLoadError ensures a malformed .plan-bender.json
// doesn't silently skip the plan-file walk. The walk must still run against
// the default plansDir and a warning must reach the user.
func TestMigrate_FallsBackOnConfigLoadError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"),
		[]byte(`{ this is not valid json`), 0o644))

	planDir := filepath.Join(dir, ".plan-bender", "plans", "demo")
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"),
		[]byte("name: Demo\nslug: demo\n"), 0o644))

	cmd := NewMigrateCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	// Warning reached the user.
	assert.Contains(t, out.String(), "warning:")
	// Walk still happened — prd.yaml converted despite the broken config.
	_, err := os.Stat(filepath.Join(planDir, "prd.json"))
	assert.NoError(t, err, "prd.json must exist — walk must have run despite config load failure")
}

// TestMigrate_PreservesBareColonListItems guards against the regression where
// yaml.Unmarshal renders `- M1: foo` items as single-key maps that the strict
// JSON decoder rejects. ProseList existed precisely to flatten these back into
// "M1: foo" strings; migrate must preserve that contract.
func TestMigrate_PreservesBareColonListItems(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	planDir := filepath.Join(dir, ".plan-bender", "plans", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(planDir, "issues"), 0o755))

	prdYAML := `name: Demo
slug: demo
status: active
created: 2026-05-11
updated: 2026-05-11
description: A test PRD
why: To prove migration handles bare-colon items
outcome: It works
decisions:
  - M1: introduce widget
  - M2: ship widget
risks:
  - Latency: spikes under load
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prdYAML), 0o644))

	issueYAML := `id: 1
slug: thing
name: Build thing
track: intent
status: backlog
priority: high
points: 2
created: 2026-05-11
updated: 2026-05-11
outcome: Thing built
scope: Build the thing
acceptance_criteria:
  - Works: when the user clicks
  - Documented: in the README
steps:
  - Step 1: design
  - Step 2: implement
`
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "issues", "1-thing.yaml"),
		[]byte(issueYAML), 0o644))

	cmd := NewMigrateCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	prdData, err := os.ReadFile(filepath.Join(planDir, "prd.json"))
	require.NoError(t, err)
	var prd schema.PRD
	dec := json.NewDecoder(bytes.NewReader(prdData))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&prd), "strict decode must accept migrated PRD JSON: %s", string(prdData))
	assert.Equal(t, []string{"M1: introduce widget", "M2: ship widget"}, prd.Decisions)
	assert.Equal(t, []string{"Latency: spikes under load"}, prd.Risks)

	issueData, err := os.ReadFile(filepath.Join(planDir, "issues", "1-thing.json"))
	require.NoError(t, err)
	var issue schema.Issue
	dec = json.NewDecoder(bytes.NewReader(issueData))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&issue), "strict decode must accept migrated issue JSON: %s", string(issueData))
	assert.Equal(t, []string{"Works: when the user clicks", "Documented: in the README"}, issue.AcceptanceCriteria)
	assert.Equal(t, []string{"Step 1: design", "Step 2: implement"}, issue.Steps)
}

// TestMigrate_UpdatesGitignoreEntry guards against the secrets exposure:
// setup.go used to write `.plan-bender.local.yaml` to .gitignore; migrate
// produces `.plan-bender.local.json` (containing Linear API keys) which
// the stale entry no longer covers.
func TestMigrate_UpdatesGitignoreEntry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath,
		[]byte("node_modules/\n.plan-bender/\n.plan-bender.local.yaml\n.env\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.local.yaml"),
		[]byte("linear:\n  api_key: secret\n"), 0o644))

	cmd := NewMigrateCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	updated, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Contains(t, string(updated), ".plan-bender.local.json")
	assert.NotContains(t, string(updated), ".plan-bender.local.yaml",
		"stale yaml line must be replaced — secrets file would otherwise be committable")
	assert.Contains(t, string(updated), "node_modules/")
	assert.Contains(t, string(updated), ".env")
}

// TestMigrate_GitignoreIdempotent ensures migrate doesn't double-add the json
// entry when run again (or when the user already had it).
func TestMigrate_GitignoreIdempotent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath,
		[]byte(".plan-bender.local.json\n"), 0o644))

	cmd := NewMigrateCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	updated, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.Equal(t, ".plan-bender.local.json\n", string(updated),
		"gitignore must be untouched when only the json entry is present")
}

// TestMigrate_GitignoreReplacesBothWithJsonOnly drops the stale yaml line
// when the json entry already exists alongside.
func TestMigrate_GitignoreReplacesBothWithJsonOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath,
		[]byte(".plan-bender.local.yaml\n.plan-bender.local.json\n"), 0o644))

	cmd := NewMigrateCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	updated, err := os.ReadFile(gitignorePath)
	require.NoError(t, err)
	assert.NotContains(t, string(updated), ".plan-bender.local.yaml")
	assert.Contains(t, string(updated), ".plan-bender.local.json")
}

// TestMigrate_LocalConfigGetsRestrictedMode guards against the regression where
// `.plan-bender.local.yaml` (Linear API key) was migrated to a 0o644 json — a
// world-readable secret file. Other config tiers stay at 0o644.
func TestMigrate_LocalConfigGetsRestrictedMode(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.yaml"),
		[]byte("max_points: 5\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.local.yaml"),
		[]byte("linear:\n  api_key: sk-secret\n"), 0o600))

	cmd := NewMigrateCmd()
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	localInfo, err := os.Stat(filepath.Join(dir, ".plan-bender.local.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), localInfo.Mode().Perm(),
		".plan-bender.local.json must be owner-only")

	projectInfo, err := os.Stat(filepath.Join(dir, ".plan-bender.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), projectInfo.Mode().Perm(),
		".plan-bender.json stays world-readable")
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
