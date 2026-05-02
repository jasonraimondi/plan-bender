package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
)

const retryIssueYAML = `id: 4
slug: ship-cli
name: Ship the CLI
track: intent
status: blocked
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2024-01-01"
tdd: true
outcome: Shipped
scope: Small
acceptance_criteria: ["It ships"]
steps: ["Target — ships"]
use_cases: ["UC-1"]
notes: subprocess timed out after 30m
`

func setupRetryPlan(t *testing.T, status string, withNotes bool) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	plansDir := filepath.Join(dir, ".plan-bender", "plans", "ship")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "prd.yaml"), []byte("name: Ship\nslug: ship\nstatus: active\n"), 0o644))

	body := retryIssueYAML
	if status != "" {
		body = strings.Replace(body, "status: blocked", "status: "+status, 1)
	}
	if !withNotes {
		body = strings.Replace(body, "notes: subprocess timed out after 30m\n", "", 1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "issues", "4-ship-cli.yaml"), []byte(body), 0o644))
	return dir
}

func loadRetryIssue(t *testing.T, dir string) schema.IssueYaml {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "plans", "ship", "issues", "4-ship-cli.yaml"))
	require.NoError(t, err)
	var issue schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &issue))
	return issue
}

func TestRetry_FlipsBlockedToTodo(t *testing.T) {
	dir := setupRetryPlan(t, "", true)

	cmd := NewRetryCmd()
	cmd.SetArgs([]string{"ship", "4"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "blocked → todo")

	issue := loadRetryIssue(t, dir)
	assert.Equal(t, "todo", issue.Status)
	require.NotNil(t, issue.Notes, "owner appends a structured note on transition")
	assert.Contains(t, *issue.Notes, "blocked→todo: retry")
	assert.Contains(t, *issue.Notes, "subprocess timed out after 30m", "prior failure context preserved")
	assert.Equal(t, time.Now().Format("2006-01-02"), issue.Updated)
}

func TestRetry_RefusesNonBlocked(t *testing.T) {
	for _, st := range []string{"in-progress", "in-review", "done", "canceled"} {
		t.Run(st, func(t *testing.T) {
			dir := setupRetryPlan(t, st, true)

			cmd := NewRetryCmd()
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
			assert.Contains(t, agentErr.Error(), "not blocked")

			issue := loadRetryIssue(t, dir)
			assert.Equal(t, st, issue.Status, "status should not change on refusal")
			require.NotNil(t, issue.Notes, "notes should not change on refusal")
			assert.Equal(t, "subprocess timed out after 30m", *issue.Notes)
		})
	}
}

func TestRetry_AlreadyTodoIsIdempotent(t *testing.T) {
	dir := setupRetryPlan(t, "todo", true)

	cmd := NewRetryCmd()
	cmd.SetArgs([]string{"ship", "4"})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute(), "already-todo is a no-op success")

	assert.Contains(t, out.String(), "already todo")

	issue := loadRetryIssue(t, dir)
	assert.Equal(t, "todo", issue.Status)
	require.NotNil(t, issue.Notes, "notes untouched on idempotent path")
	assert.Equal(t, "subprocess timed out after 30m", *issue.Notes)
}

func TestRetry_UnknownIssue(t *testing.T) {
	setupRetryPlan(t, "", true)

	cmd := NewRetryCmd()
	cmd.SetArgs([]string{"ship", "99"})
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	require.Error(t, err)

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, ErrPlanNotFound, agentErr.Code)
}

func TestRetry_AgentModeJSON(t *testing.T) {
	dir := setupRetryPlan(t, "", true)

	root := NewAgentRootCmd("test")
	root.SetArgs([]string{"retry", "ship", "4"})
	var out strings.Builder
	root.SetOut(&out)
	require.NoError(t, root.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &got))
	assert.Equal(t, "ok", got["status"])
	assert.EqualValues(t, 4, got["id"])
	assert.Equal(t, "todo", got["new_status"])

	issue := loadRetryIssue(t, dir)
	assert.Equal(t, "todo", issue.Status)
	require.NotNil(t, issue.Notes)
	assert.Contains(t, *issue.Notes, "blocked→todo: retry")
}

// TestRetry_ConcurrentRaceSurfacesCASMismatch: run retry (blocked→todo) and a
// competing transition (blocked→in-progress) on the same issue from two
// goroutines. The plan-wide flock serializes them; the loser observes the new
// state and surfaces a CAS mismatch.
func TestRetry_ConcurrentRaceSurfacesCASMismatch(t *testing.T) {
	dir := setupRetryPlan(t, "", true)
	cfg, err := config.Load(dir)
	require.NoError(t, err)

	var (
		wg         sync.WaitGroup
		retryErr   error
		competeErr error
	)
	wg.Add(2)

	go func() {
		defer wg.Done()
		cmd := NewRetryCmd()
		cmd.SetArgs([]string{"ship", "4"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		retryErr = cmd.Execute()
	}()

	go func() {
		defer wg.Done()
		owner := dispatch.NewProdStatusOwner(cfg.PlansDir)
		competeErr = owner.Transition(context.Background(), "ship", 4,
			[]status.Status{status.StatusBlocked}, status.StatusInProgress, "manual resume")
	}()

	wg.Wait()

	winners := 0
	casMismatches := 0
	if retryErr == nil {
		winners++
	} else {
		var agentErr *AgentError
		require.ErrorAs(t, retryErr, &agentErr, "retry error must be AgentError")
		assert.Equal(t, ErrValidationFailed, agentErr.Code)
		assert.Contains(t, agentErr.Error(), "not blocked")
		casMismatches++
	}
	if competeErr == nil {
		winners++
	} else {
		var casErr *status.ErrCASMismatch
		require.ErrorAs(t, competeErr, &casErr, "compete error must be CAS mismatch")
		casMismatches++
	}
	assert.Equal(t, 1, winners, "exactly one transition wins")
	assert.Equal(t, 1, casMismatches, "exactly one transition surfaces CAS mismatch")

	final := loadRetryIssue(t, dir)
	assert.Contains(t, []string{"todo", "in-progress"}, final.Status,
		"final status reflects the winner")
}
