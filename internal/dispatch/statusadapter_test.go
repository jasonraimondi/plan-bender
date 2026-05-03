package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
)

// adapterTestCfg returns a config that accepts the validAdapterIssue fixtures
// when planrepo.Commit runs validation.
func adapterTestCfg() config.Config {
	return config.Config{
		Tracks:         []string{"data", "rules"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"},
		MaxPoints:      8,
	}
}

const adapterValidPrd = `name: Demo
slug: demo
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: A test
why: Testing
outcome: Tests pass
`

// validAdapterIssue returns an issue that passes planrepo.Commit validation
// under adapterTestCfg.
func validAdapterIssue(id int, slug, statusStr string) schema.IssueYaml {
	return schema.IssueYaml{
		ID:                 id,
		Slug:               slug,
		Name:               "Issue " + slug,
		Track:              "data",
		Status:             statusStr,
		Priority:           "high",
		Points:             1,
		Labels:             []string{},
		BlockedBy:          []int{},
		Blocking:           []int{},
		Created:            "2026-01-01",
		Updated:            "2026-01-02",
		Outcome:            "ok",
		Scope:              "small",
		AcceptanceCriteria: []string{},
		Steps:              []string{},
		UseCases:           []string{},
	}
}

// writeValidPlan seeds plansDir with a valid PRD and the given issues at
// canonical {id}-{slug}.yaml paths.
func writeValidPlan(t *testing.T, plansDir, slug string, issues ...schema.IssueYaml) {
	t.Helper()
	planDir := filepath.Join(plansDir, slug)
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(adapterValidPrd), 0o644))
	for _, iss := range issues {
		data, err := yaml.Marshal(iss)
		require.NoError(t, err)
		path := filepath.Join(issuesDir, fmt.Sprintf("%d-%s.yaml", iss.ID, iss.Slug))
		require.NoError(t, os.WriteFile(path, data, 0o644))
	}
}

func loadIssueFile(t *testing.T, plansDir, slug string, id int, issueSlug string) schema.IssueYaml {
	t.Helper()
	path := filepath.Join(plansDir, slug, "issues", fmt.Sprintf("%d-%s.yaml", id, issueSlug))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var iss schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &iss))
	return iss
}

// TestProdStatusOwner_TransitionPersistsThroughPlanrepo asserts that
// owner.Transition through the prod adapter writes the new status atomically
// to disk via the planrepo session path. The reload check proves the write
// reached disk under the session lock.
func TestProdStatusOwner_TransitionPersistsThroughPlanrepo(t *testing.T) {
	plansDir := t.TempDir()
	writeValidPlan(t, plansDir, "demo", validAdapterIssue(7, "alpha", "todo"))

	owner := NewProdStatusOwner(plansDir, adapterTestCfg())
	require.NoError(t, owner.Transition(context.Background(), "demo", 7,
		[]status.Status{status.StatusTodo}, status.StatusInProgress, ""))

	post := loadIssueFile(t, plansDir, "demo", 7, "alpha")
	assert.Equal(t, "in-progress", post.Status)
}

// TestProdStatusOwner_ClaimStampsBranchAndStatus asserts that owner.Claim
// through the prod adapter persists both the new status and the branch field.
func TestProdStatusOwner_ClaimStampsBranchAndStatus(t *testing.T) {
	plansDir := t.TempDir()
	writeValidPlan(t, plansDir, "demo", validAdapterIssue(7, "alpha", "backlog"))

	owner := NewProdStatusOwner(plansDir, adapterTestCfg())
	require.NoError(t, owner.Claim(context.Background(), "demo", 7,
		"user/demo--7-alpha", "worktree create"))

	post := loadIssueFile(t, plansDir, "demo", 7, "alpha")
	assert.Equal(t, "in-progress", post.Status)
	require.NotNil(t, post.Branch)
	assert.Equal(t, "user/demo--7-alpha", *post.Branch)
}

// TestProdStatusOwner_PreflightValidationRejectsCommitWhenIssueInvalid asserts
// the planrepo commit path enforces validation: if a track on disk is no
// longer in cfg.Tracks (e.g. user removed it from .plan-bender.yaml), the
// commit refuses rather than silently writing a now-invalid issue.
func TestProdStatusOwner_PreflightValidationRejectsCommitWhenIssueInvalid(t *testing.T) {
	plansDir := t.TempDir()
	iss := validAdapterIssue(7, "alpha", "todo")
	iss.Track = "intent" // not in adapterTestCfg().Tracks
	writeValidPlan(t, plansDir, "demo", iss)

	owner := NewProdStatusOwner(plansDir, adapterTestCfg())
	err := owner.Transition(context.Background(), "demo", 7,
		[]status.Status{status.StatusTodo}, status.StatusInProgress, "")
	require.Error(t, err, "commit must reject writes when on-disk issue would fail validation")
}

// TestProdStatusOwner_ConcurrentTransitionsSerializeViaSessionLock asserts
// that concurrent owner.Transition calls funnel through the planrepo session
// lock: exactly one transition succeeds, the rest see the post-write state
// and observe ErrAlreadyInState — without serialization multiple goroutines
// would race past the CAS check and produce duplicate Saves.
func TestProdStatusOwner_ConcurrentTransitionsSerializeViaSessionLock(t *testing.T) {
	plansDir := t.TempDir()
	writeValidPlan(t, plansDir, "demo", validAdapterIssue(1, "x", "todo"))

	owner := NewProdStatusOwner(plansDir, adapterTestCfg())

	const N = 16
	var wg sync.WaitGroup
	var success, alreadyInState atomic.Int32

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := owner.Transition(context.Background(), "demo", 1,
				[]status.Status{status.StatusTodo}, status.StatusInProgress, "")
			switch {
			case err == nil:
				success.Add(1)
			case errors.Is(err, status.ErrAlreadyInState):
				alreadyInState.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, success.Load(), "exactly one Transition wins under serialized session lock")
	assert.EqualValues(t, N-1, alreadyInState.Load(), "remaining callers observe post-write state")

	post := loadIssueFile(t, plansDir, "demo", 1, "x")
	assert.Equal(t, "in-progress", post.Status)
}

// TestProdStatusOwner_OpenSessionMissingPlanIsAnError asserts that opening
// a session against a non-existent plan returns an error rather than panicking.
func TestProdStatusOwner_OpenSessionMissingPlanIsAnError(t *testing.T) {
	plansDir := t.TempDir()
	owner := NewProdStatusOwner(plansDir, adapterTestCfg())

	err := owner.Transition(context.Background(), "nope", 1,
		[]status.Status{status.StatusTodo}, status.StatusInProgress, "")
	require.Error(t, err)
}

// TestProdStatusOwner_TransitionAndClaimReleaseLockBetweenCalls asserts the
// session lock is released after each Owner call: a follow-up call cannot
// hang waiting on the previous session.
func TestProdStatusOwner_TransitionAndClaimReleaseLockBetweenCalls(t *testing.T) {
	plansDir := t.TempDir()
	writeValidPlan(t, plansDir, "demo", validAdapterIssue(1, "x", "todo"))
	owner := NewProdStatusOwner(plansDir, adapterTestCfg())

	done := make(chan error, 1)
	go func() {
		ctx := context.Background()
		if err := owner.Transition(ctx, "demo", 1,
			[]status.Status{status.StatusTodo}, status.StatusInProgress, ""); err != nil {
			done <- err
			return
		}
		// If the first session didn't release, this Claim would deadlock.
		done <- owner.Claim(ctx, "demo", 1, "user/demo--1-x", "")
	}()

	select {
	case err := <-done:
		require.True(t, err == nil || errors.Is(err, status.ErrAlreadyInState),
			"expected nil or ErrAlreadyInState, got %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Transition + Claim sequence deadlocked — session lock not released")
	}
}
