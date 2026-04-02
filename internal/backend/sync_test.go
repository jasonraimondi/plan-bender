package backend

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mockBackend implements Backend with per-method function fields.
type mockBackend struct {
	createProject func(ctx context.Context, prd *schema.PrdYaml) (RemoteProject, error)
	createIssue   func(ctx context.Context, issue *schema.IssueYaml, projectID string) (RemoteIssue, error)
	updateIssue   func(ctx context.Context, issue *schema.IssueYaml) (RemoteIssue, error)
	pullIssue     func(ctx context.Context, remoteID string) (RemoteIssue, error)
	pullProject   func(ctx context.Context, projectID string) (PullProjectResult, error)
}

func (m *mockBackend) CreateProject(ctx context.Context, prd *schema.PrdYaml) (RemoteProject, error) {
	return m.createProject(ctx, prd)
}
func (m *mockBackend) CreateIssue(ctx context.Context, issue *schema.IssueYaml, projectID string) (RemoteIssue, error) {
	return m.createIssue(ctx, issue, projectID)
}
func (m *mockBackend) UpdateIssue(ctx context.Context, issue *schema.IssueYaml) (RemoteIssue, error) {
	return m.updateIssue(ctx, issue)
}
func (m *mockBackend) PullIssue(ctx context.Context, remoteID string) (RemoteIssue, error) {
	return m.pullIssue(ctx, remoteID)
}
func (m *mockBackend) PullProject(ctx context.Context, projectID string) (PullProjectResult, error) {
	return m.pullProject(ctx, projectID)
}

// setupSyncTest creates a temp dir with PRD and issues, returns a PlanStore and the dir path.
func setupSyncTest(t *testing.T, prd *schema.PrdYaml, issues []*schema.IssueYaml) *PlanStore {
	t.Helper()
	dir := t.TempDir()
	slug := prd.Slug

	store := NewProdPlanStore(dir)

	require.NoError(t, store.WritePrd(slug, prd))
	for _, issue := range issues {
		require.NoError(t, store.WriteIssue(slug, issue))
	}
	return store
}

func readIssueFromDisk(t *testing.T, store *PlanStore, slug string, id int, issueSlug string) schema.IssueYaml {
	t.Helper()
	path := filepath.Join(store.root, slug, "issues", fmt.Sprintf("%d-%s.yaml", id, issueSlug))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var issue schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &issue))
	return issue
}

func TestSyncPush_AllCreate(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	issues := []*schema.IssueYaml{testIssue(1), testIssue(2), testIssue(3)}
	issues[1].Slug = "issue-two"
	issues[1].Name = "Issue two"
	issues[2].Slug = "issue-three"
	issues[2].Name = "Issue three"

	store := setupSyncTest(t, prd, issues)

	createCount := 0
	be := &mockBackend{
		createIssue: func(_ context.Context, issue *schema.IssueYaml, projectID string) (RemoteIssue, error) {
			createCount++
			return RemoteIssue{ID: fmt.Sprintf("lin-%d", issue.ID)}, nil
		},
	}

	result, err := SyncPush(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 3, result.Created)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)
	assert.Equal(t, 3, createCount)

	// Verify linear_id was written back for each issue
	i1 := readIssueFromDisk(t, store, "test", 1, "test-issue")
	assert.Equal(t, "lin-1", *i1.LinearID)

	i2 := readIssueFromDisk(t, store, "test", 2, "issue-two")
	assert.Equal(t, "lin-2", *i2.LinearID)

	i3 := readIssueFromDisk(t, store, "test", 3, "issue-three")
	assert.Equal(t, "lin-3", *i3.LinearID)
}

func TestSyncPush_AllUpdate(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	linID1, linID2, linID3 := "existing-1", "existing-2", "existing-3"
	issues := []*schema.IssueYaml{testIssue(1), testIssue(2), testIssue(3)}
	issues[0].LinearID = &linID1
	issues[1].Slug = "issue-two"
	issues[1].LinearID = &linID2
	issues[2].Slug = "issue-three"
	issues[2].LinearID = &linID3

	store := setupSyncTest(t, prd, issues)

	updateCount := 0
	be := &mockBackend{
		updateIssue: func(_ context.Context, issue *schema.IssueYaml) (RemoteIssue, error) {
			updateCount++
			return RemoteIssue{ID: *issue.LinearID}, nil
		},
	}

	result, err := SyncPush(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 3, result.Updated)
	assert.Empty(t, result.Errors)
	assert.Equal(t, 3, updateCount)
}

func TestSyncPush_PartialFailure(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	issues := []*schema.IssueYaml{testIssue(1), testIssue(2), testIssue(3)}
	issues[1].Slug = "issue-two"
	issues[2].Slug = "issue-three"

	store := setupSyncTest(t, prd, issues)

	be := &mockBackend{
		createIssue: func(_ context.Context, issue *schema.IssueYaml, _ string) (RemoteIssue, error) {
			if issue.ID == 2 {
				return RemoteIssue{}, fmt.Errorf("api error")
			}
			return RemoteIssue{ID: fmt.Sprintf("lin-%d", issue.ID)}, nil
		},
	}

	result, err := SyncPush(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 0, result.Updated)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, 2, result.Errors[0].IssueID)
	assert.Contains(t, result.Errors[0].Err.Error(), "api error")

	// Verify linear_id written for successes only
	i1 := readIssueFromDisk(t, store, "test", 1, "test-issue")
	assert.NotNil(t, i1.LinearID)
	assert.Equal(t, "lin-1", *i1.LinearID)

	i3 := readIssueFromDisk(t, store, "test", 3, "issue-three")
	assert.NotNil(t, i3.LinearID)
	assert.Equal(t, "lin-3", *i3.LinearID)

	// Issue 2 should NOT have linear_id
	i2 := readIssueFromDisk(t, store, "test", 2, "issue-two")
	assert.Nil(t, i2.LinearID)
}

func TestSyncPush_Idempotent(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	linID := "existing-1"
	issues := []*schema.IssueYaml{testIssue(1), testIssue(2)}
	issues[0].LinearID = &linID // already synced
	issues[1].Slug = "issue-two" // not yet synced

	store := setupSyncTest(t, prd, issues)

	var createCalls, updateCalls []int
	be := &mockBackend{
		createIssue: func(_ context.Context, issue *schema.IssueYaml, _ string) (RemoteIssue, error) {
			createCalls = append(createCalls, issue.ID)
			return RemoteIssue{ID: fmt.Sprintf("lin-%d", issue.ID)}, nil
		},
		updateIssue: func(_ context.Context, issue *schema.IssueYaml) (RemoteIssue, error) {
			updateCalls = append(updateCalls, issue.ID)
			return RemoteIssue{ID: *issue.LinearID}, nil
		},
	}

	result, err := SyncPush(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)

	// Issue 1 (has linear_id) → UpdateIssue, not CreateIssue
	assert.Equal(t, []int{1}, updateCalls)
	// Issue 2 (no linear_id) → CreateIssue
	assert.Equal(t, []int{2}, createCalls)
}

func TestSyncPush_CreatesProject(t *testing.T) {
	prd := testPrd()
	// No Linear ref — should trigger CreateProject

	store := setupSyncTest(t, prd, nil)

	projectCreated := false
	be := &mockBackend{
		createProject: func(_ context.Context, prd *schema.PrdYaml) (RemoteProject, error) {
			projectCreated = true
			return RemoteProject{ID: "new-proj"}, nil
		},
	}

	result, err := SyncPush(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.True(t, projectCreated)
	assert.Equal(t, 0, result.Created)

	// Verify project_id was written back to PRD
	updatedPrd, err := store.ReadPrd("test")
	require.NoError(t, err)
	require.NotNil(t, updatedPrd.Linear)
	assert.Equal(t, "new-proj", updatedPrd.Linear.ProjectID)
}

// --- SyncPull tests ---

func TestSyncPull_StatusUpdate(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	linID := "lin-1"
	issue := testIssue(1)
	issue.LinearID = &linID
	issue.Status = "backlog"

	store := setupSyncTest(t, prd, []*schema.IssueYaml{issue})

	be := &mockBackend{
		pullProject: func(_ context.Context, _ string) (PullProjectResult, error) {
			return PullProjectResult{
				Project: RemoteProject{ID: "proj-1"},
				Issues: []RemoteIssue{
					{ID: "lin-1", Status: "in-progress"},
				},
			}, nil
		},
	}

	result, err := SyncPull(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)

	updated := readIssueFromDisk(t, store, "test", 1, "test-issue")
	assert.Equal(t, "in-progress", updated.Status)
}

func TestSyncPull_PriorityAndAssignee(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	linID := "lin-1"
	issue := testIssue(1)
	issue.LinearID = &linID
	issue.Priority = "low"
	issue.Assignee = nil

	store := setupSyncTest(t, prd, []*schema.IssueYaml{issue})

	be := &mockBackend{
		pullProject: func(_ context.Context, _ string) (PullProjectResult, error) {
			return PullProjectResult{
				Project: RemoteProject{ID: "proj-1"},
				Issues: []RemoteIssue{
					{ID: "lin-1", Status: "backlog", Priority: "urgent", Assignee: "alice"},
				},
			}, nil
		},
	}

	result, err := SyncPull(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Empty(t, result.Errors)

	updated := readIssueFromDisk(t, store, "test", 1, "test-issue")
	assert.Equal(t, "urgent", updated.Priority)
	require.NotNil(t, updated.Assignee)
	assert.Equal(t, "alice", *updated.Assignee)
}

func TestSyncPull_SkipWithoutLinearID(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}

	issue := testIssue(1)
	// No linear_id — should be skipped

	store := setupSyncTest(t, prd, []*schema.IssueYaml{issue})

	be := &mockBackend{
		pullProject: func(_ context.Context, _ string) (PullProjectResult, error) {
			return PullProjectResult{
				Project: RemoteProject{ID: "proj-1"},
				Issues:  []RemoteIssue{{ID: "lin-1", Status: "done"}},
			}, nil
		},
	}

	result, err := SyncPull(context.Background(), store, be, "test")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Updated)
	assert.Empty(t, result.Errors)

	// Issue should be unchanged
	unchanged := readIssueFromDisk(t, store, "test", 1, "test-issue")
	assert.Equal(t, "backlog", unchanged.Status)
}

// --- Marshal/write error propagation tests ---

func TestSyncPush_WriteIssueError(t *testing.T) {
	// Set up a store where WriteIssue fails (simulating marshal/write failure)
	dir := t.TempDir()
	realStore := NewProdPlanStore(dir)

	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}
	require.NoError(t, realStore.WritePrd("test", prd))

	issue := testIssue(1)
	require.NoError(t, realStore.WriteIssue("test", issue))

	// Create a store with a failing write func (reads from real disk, writes fail)
	failStore := NewPlanStore(dir, prodFS(dir), func(_ string, _ []byte, _ fs.FileMode) error {
		return fmt.Errorf("write failed")
	}, prodMkdir)

	be := &mockBackend{
		createIssue: func(_ context.Context, _ *schema.IssueYaml, _ string) (RemoteIssue, error) {
			return RemoteIssue{ID: "lin-1"}, nil
		},
	}

	result, err := SyncPush(context.Background(), failStore, be, "test")
	require.NoError(t, err) // SyncPush itself succeeds (continue-on-error)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, 1, result.Errors[0].IssueID)
	assert.Contains(t, result.Errors[0].Err.Error(), "write failed")
	assert.Equal(t, 0, result.Created) // not counted as created since write-back failed
}

func TestSyncPull_WriteIssueError(t *testing.T) {
	dir := t.TempDir()
	realStore := NewProdPlanStore(dir)

	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}
	require.NoError(t, realStore.WritePrd("test", prd))

	linID := "lin-1"
	issue := testIssue(1)
	issue.LinearID = &linID
	require.NoError(t, realStore.WriteIssue("test", issue))

	// Store that reads from real disk but fails on write
	failStore := NewPlanStore(dir, prodFS(dir), func(_ string, _ []byte, _ fs.FileMode) error {
		return fmt.Errorf("disk error")
	}, prodMkdir)

	be := &mockBackend{
		pullProject: func(_ context.Context, _ string) (PullProjectResult, error) {
			return PullProjectResult{
				Project: RemoteProject{ID: "proj-1"},
				Issues:  []RemoteIssue{{ID: "lin-1", Status: "done"}},
			}, nil
		},
	}

	result, err := SyncPull(context.Background(), failStore, be, "test")
	require.NoError(t, err) // SyncPull itself succeeds (continue-on-error)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, 1, result.Errors[0].IssueID)
	assert.Contains(t, result.Errors[0].Err.Error(), "disk error")
	assert.Equal(t, 0, result.Updated) // not counted as updated since write failed
}
