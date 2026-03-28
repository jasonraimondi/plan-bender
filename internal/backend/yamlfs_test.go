package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBackend(t *testing.T) (Backend, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.PlansDir = dir
	return NewYAMLFS(cfg), dir
}

func testPrd() *schema.PrdYaml {
	return &schema.PrdYaml{
		Name:        "Test",
		Slug:        "test",
		Status:      "active",
		Created:     "2026-03-26",
		Updated:     "2026-03-26",
		Description: "Test PRD",
		Why:         "Testing",
		Outcome:     "Tests pass",
	}
}

func testIssue(id int) *schema.IssueYaml {
	return &schema.IssueYaml{
		ID:                 id,
		Slug:               "test-issue",
		Name:               "Test issue",
		Track:              "intent",
		Status:             "backlog",
		Priority:           "high",
		Points:             2,
		Labels:             []string{"AFK"},
		BlockedBy:          []int{},
		Blocking:           []int{},
		Created:            "2026-03-26",
		Updated:            "2026-03-26",
		Outcome:            "Done",
		Scope:              "Do it",
		AcceptanceCriteria: []string{"Works"},
		Steps:              []string{"Step 1"},
		UseCases:           []string{},
	}
}

func TestFactory_YAMLFS(t *testing.T) {
	cfg := config.Defaults()
	cfg.PlansDir = t.TempDir()
	b, err := New(context.Background(), cfg)
	require.NoError(t, err)
	assert.NotNil(t, b)
}

func TestFactory_UnknownBackend(t *testing.T) {
	cfg := config.Defaults()
	cfg.Backend = "nope"
	_, err := New(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend")
}

func TestCreateProject(t *testing.T) {
	b, dir := testBackend(t)
	ctx := context.Background()

	result, err := b.CreateProject(ctx, testPrd())
	require.NoError(t, err)
	assert.Equal(t, "test", result.ID)
	assert.Equal(t, "Test", result.Name)

	// prd.yaml exists
	_, err = os.Stat(filepath.Join(dir, "test", "prd.yaml"))
	assert.NoError(t, err)
	// issues dir exists
	_, err = os.Stat(filepath.Join(dir, "test", "issues"))
	assert.NoError(t, err)
}

func TestCreateIssue(t *testing.T) {
	b, dir := testBackend(t)
	ctx := context.Background()

	_, err := b.CreateProject(ctx, testPrd())
	require.NoError(t, err)

	result, err := b.CreateIssue(ctx, testIssue(1), "test")
	require.NoError(t, err)
	assert.Equal(t, "1", result.ID)
	assert.Equal(t, "Test issue", result.Title)

	path := filepath.Join(dir, "test", "issues", "1-test-issue.yaml")
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestUpdateIssue(t *testing.T) {
	b, _ := testBackend(t)
	ctx := context.Background()

	_, err := b.CreateProject(ctx, testPrd())
	require.NoError(t, err)
	_, err = b.CreateIssue(ctx, testIssue(1), "test")
	require.NoError(t, err)

	updated := testIssue(1)
	updated.Status = "in-progress"
	result, err := b.UpdateIssue(ctx, updated)
	require.NoError(t, err)
	assert.Equal(t, "in-progress", result.Status)
}

func TestPullIssue(t *testing.T) {
	b, _ := testBackend(t)
	ctx := context.Background()

	_, err := b.CreateProject(ctx, testPrd())
	require.NoError(t, err)
	_, err = b.CreateIssue(ctx, testIssue(1), "test")
	require.NoError(t, err)

	result, err := b.PullIssue(ctx, "test/1")
	require.NoError(t, err)
	assert.Equal(t, "1", result.ID)
	assert.Equal(t, "Test issue", result.Title)
	assert.Equal(t, "backlog", result.Status)
}

func TestPullProject(t *testing.T) {
	b, _ := testBackend(t)
	ctx := context.Background()

	_, err := b.CreateProject(ctx, testPrd())
	require.NoError(t, err)
	_, err = b.CreateIssue(ctx, testIssue(1), "test")
	require.NoError(t, err)

	issue2 := testIssue(2)
	issue2.Slug = "second-issue"
	issue2.Name = "Second"
	_, err = b.CreateIssue(ctx, issue2, "test")
	require.NoError(t, err)

	result, err := b.PullProject(ctx, "test")
	require.NoError(t, err)
	assert.Equal(t, "Test", result.Project.Name)
	assert.Len(t, result.Issues, 2)
}

func TestPullIssue_InvalidFormat(t *testing.T) {
	b, _ := testBackend(t)
	_, err := b.PullIssue(context.Background(), "bad-format")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid remoteId")
}
