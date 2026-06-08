package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

func setupParkPlan(t *testing.T, status string) string {
	t.Helper()
	dir := t.TempDir()
	chdir(t, dir)
	plansDir := filepath.Join(dir, ".plan-bender", "plans", "ship")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.json"), []byte(validShipPrd), 0o644))

	body := strings.Replace(retryIssueJSON, `"status": "blocked"`, `"status": "`+status+`"`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "4-ship-cli.json"), []byte(body), 0o644))
	return dir
}

func loadParkIssue(t *testing.T, dir string) schema.Issue {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "ship", "issues", "4-ship-cli.json"))
	require.NoError(t, err)
	var issue schema.Issue
	require.NoError(t, json.Unmarshal(data, &issue))
	return issue
}

func TestPark_FlipsInProgressToNeedsInput(t *testing.T) {
	dir := setupParkPlan(t, "in-progress")

	cmd := NewParkCmd()
	cmd.SetArgs([]string{"ship", "4"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "in-progress → needs-input")

	issue := loadParkIssue(t, dir)
	assert.Equal(t, "needs-input", issue.Status)
	require.NotNil(t, issue.Notes, "owner appends a structured note on transition")
	assert.Contains(t, *issue.Notes, "in-progress→needs-input: park")
}

func TestPark_AgentModeJSON(t *testing.T) {
	dir := setupParkPlan(t, "in-progress")

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"park", "ship", "4"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	assert.Equal(t, "ok", got["status"])
	assert.EqualValues(t, 4, got["id"])
	assert.Equal(t, "needs-input", got["new_status"])

	issue := loadParkIssue(t, dir)
	assert.Equal(t, "needs-input", issue.Status)
}

func TestPark_RefusesNonInProgress(t *testing.T) {
	for _, st := range []string{"todo", "in-review", "blocked", "done"} {
		t.Run(st, func(t *testing.T) {
			dir := setupParkPlan(t, st)

			cmd := NewParkCmd()
			cmd.SetArgs([]string{"ship", "4"})
			var out, errOut strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			err := cmd.Execute()
			require.Error(t, err)

			var agentErr *AgentError
			require.ErrorAs(t, err, &agentErr)
			assert.Equal(t, ErrValidationFailed, agentErr.Code)
			assert.Contains(t, agentErr.Error(), st, "error message reports current state")
			assert.Contains(t, agentErr.Error(), "not in-progress")

			issue := loadParkIssue(t, dir)
			assert.Equal(t, st, issue.Status, "status should not change on refusal")
		})
	}
}

func TestPark_AlreadyNeedsInputIsIdempotent(t *testing.T) {
	dir := setupParkPlan(t, "needs-input")

	cmd := NewParkCmd()
	cmd.SetArgs([]string{"ship", "4"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute(), "already-needs-input is a no-op success")

	assert.Contains(t, out.String(), "already needs-input")

	issue := loadParkIssue(t, dir)
	assert.Equal(t, "needs-input", issue.Status)
}
