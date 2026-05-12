package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPlanDir(t *testing.T, slug string, issues []schema.Issue) string {
	t.Helper()
	dir := t.TempDir()
	planDir := filepath.Join(dir, "plans", slug)
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))

	prd := schema.PRD{
		Name: "Test", Slug: slug, Status: "active",
		Created: "2026-03-26", Updated: "2026-03-26",
		Description: "Test", Why: "Test", Outcome: "Test",
	}
	data, _ := json.Marshal(&prd)
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.json"), data, 0o644))

	for _, iss := range issues {
		data, _ := json.Marshal(&iss)
		filename := filepath.Join(issuesDir, strings.ReplaceAll(iss.Slug, " ", "-")+".json")
		require.NoError(t, os.WriteFile(filename, data, 0o644))
	}

	cfgData := []byte(`{"plans_dir": "` + filepath.Join(dir, "plans") + `/"}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"), cfgData, 0o644))

	return dir
}

func TestArchive_BlocksOnActiveIssues(t *testing.T) {
	issues := []schema.Issue{
		{ID: 1, Slug: "active", Name: "Active", Status: "in-progress", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	chdir(t, dir)

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetOut(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active issues")
}

func TestArchive_SucceedsWithForce(t *testing.T) {
	issues := []schema.Issue{
		{ID: 1, Slug: "active", Name: "Active", Status: "in-progress", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	chdir(t, dir)

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test", "--force"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "archived test")

	_, err := os.Stat(filepath.Join(dir, "plans", ".archive", "test", "prd.json"))
	assert.NoError(t, err)
}

func TestArchive_AllDoneSucceeds(t *testing.T) {
	issues := []schema.Issue{
		{ID: 1, Slug: "done-issue", Name: "Done", Status: "done", Track: "intent", Priority: "high", Points: 1, Labels: []string{}, BlockedBy: []int{}, Blocking: []int{}, Created: "2026-03-26", Updated: "2026-03-26", Outcome: "x", Scope: "x", AcceptanceCriteria: []string{}, Steps: []string{}, UseCases: []string{}},
	}
	dir := setupPlanDir(t, "test", issues)
	chdir(t, dir)

	cmd := NewArchiveCmd()
	cmd.SetArgs([]string{"test"})
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())
}
