package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func setupPlanDir(t *testing.T, slug string, issues []schema.IssueYaml) string {
	t.Helper()
	dir := t.TempDir()
	planDir := filepath.Join(dir, "plans", slug)
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	prd := schema.PrdYaml{
		Name: "Test", Slug: slug, Status: "active",
		Created: "2026-03-26", Updated: "2026-03-26",
		Description: "Test", Why: "Test", Outcome: "Test",
	}
	data, _ := yaml.Marshal(&prd)
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), data, 0o644))

	for _, iss := range issues {
		data, _ := yaml.Marshal(&iss)
		filename := filepath.Join(issuesDir, strings.ReplaceAll(iss.Slug, " ", "-")+".yaml")
		require.NoError(t, os.WriteFile(filename, data, 0o644))
	}

	// Write config pointing to this plans dir
	cfgData := []byte("plans_dir: " + filepath.Join(dir, "plans") + "/\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.yaml"), cfgData, 0o644))

	return dir
}

func TestArchive_BlocksOnActiveIssues(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Slug: "active", Name: "Active", Status: "in-progress", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	require.NoError(t, os.Chdir(dir))

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetOut(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active issues")
}

func TestArchive_SucceedsWithForce(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Slug: "active", Name: "Active", Status: "in-progress", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	require.NoError(t, os.Chdir(dir))

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test", "--force"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "archived test")

	// Verify moved to .archive/
	_, err := os.Stat(filepath.Join(dir, "plans", ".archive", "test", "prd.yaml"))
	assert.NoError(t, err)
}

func TestArchive_AllDoneSucceeds(t *testing.T) {
	issues := []schema.IssueYaml{
		{ID: 1, Slug: "done-issue", Name: "Done", Status: "done", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	require.NoError(t, os.Chdir(dir))

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())
}
