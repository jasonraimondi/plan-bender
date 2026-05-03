package planrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testCfg returns a minimal config sufficient for in-memory issue validation.
func testCfg() config.Config {
	return config.Config{
		Tracks:         []string{"data", "rules"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"},
		MaxPoints:      8,
	}
}

// validIssue returns a fully-populated valid issue for use in tests.
func validIssue(id int, slug string) schema.IssueYaml {
	return schema.IssueYaml{
		ID:                 id,
		Slug:               slug,
		Name:               "Issue " + slug,
		Track:              "data",
		Status:             "todo",
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

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func mustSnapshot(t *testing.T, s *PlanSession) Snapshot {
	t.Helper()
	snap, err := s.Snapshot()
	require.NoError(t, err)
	return snap
}

// --- Mutations ---

func TestUpdatePrd_ReflectedInSnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	prd := mustSnapshot(t, sess).PRD
	prd.Name = "Renamed Plan"
	require.NoError(t, sess.UpdatePrd(prd))

	assert.Equal(t, "Renamed Plan", mustSnapshot(t, sess).PRD.Name)
}

func TestUpdateIssue_ReflectedInSnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	updated := mustSnapshot(t, sess).Issues[0]
	updated.Status = "in-progress"
	require.NoError(t, sess.UpdateIssue(updated))

	assert.Equal(t, "in-progress", mustSnapshot(t, sess).Issues[0].Status)
}

func TestUpdateIssue_RejectsUnknownID(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	err = sess.UpdateIssue(validIssue(99, "nope"))
	require.Error(t, err)
}

func TestCreateIssue_AppearsInSnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	require.NoError(t, sess.CreateIssue(validIssue(2, "b")))

	require.Len(t, mustSnapshot(t, sess).Issues, 2)
	ids := []int{mustSnapshot(t, sess).Issues[0].ID, mustSnapshot(t, sess).Issues[1].ID}
	assert.ElementsMatch(t, []int{1, 2}, ids)
}

func TestCreateIssue_RejectsDuplicateID(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	err = sess.CreateIssue(validIssue(1, "different-slug"))
	require.Error(t, err)
}

// --- Commit preflight ---

func TestCommit_PreflightValidationFailureNoWrites(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})
	originalIssue := mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml"))
	originalPrd := mustReadFile(t, filepath.Join(plansDir, "p", "prd.yaml"))

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	bad := mustSnapshot(t, sess).Issues[0]
	bad.Slug = "" // violates required
	require.NoError(t, sess.UpdateIssue(bad))

	err = sess.Commit(testCfg())
	require.Error(t, err)

	// On-disk files unchanged.
	assert.Equal(t, originalIssue, mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml")))
	assert.Equal(t, originalPrd, mustReadFile(t, filepath.Join(plansDir, "p", "prd.yaml")))
}

func TestCommit_AlwaysValidatesEvenWhenOnDiskWasValid(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	// Cycle: 1 blocks itself via mutation.
	bad := mustSnapshot(t, sess).Issues[0]
	bad.BlockedBy = []int{1}
	require.NoError(t, sess.UpdateIssue(bad))

	err = sess.Commit(testCfg())
	require.Error(t, err, "validation must catch self-reference even if on-disk file was clean")
}

func TestValidate_RoutesThroughInMemorySnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	res := sess.Validate(testCfg())
	assert.True(t, res.Valid, "freshly-loaded valid plan must validate")

	bad := mustSnapshot(t, sess).Issues[0]
	bad.Slug = ""
	require.NoError(t, sess.UpdateIssue(bad))

	res = sess.Validate(testCfg())
	assert.False(t, res.Valid, "in-session mutation invalidating a field must surface in Validate without disk reread")
}

// --- Commit success cases ---

func TestCommit_WritesDirtyPrdAndIssues(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)

	prd := mustSnapshot(t, sess).PRD
	prd.Updated = "2026-05-03"
	require.NoError(t, sess.UpdatePrd(prd))

	iss := mustSnapshot(t, sess).Issues[0]
	iss.Status = "in-progress"
	require.NoError(t, sess.UpdateIssue(iss))

	require.NoError(t, sess.Commit(testCfg()))
	require.NoError(t, sess.Close())

	// Re-open and verify the new state is on disk.
	sess2, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess2.Close() }()
	assert.Equal(t, "2026-05-03", mustSnapshot(t, sess2).PRD.Updated)
	require.Len(t, mustSnapshot(t, sess2).Issues, 1)
	assert.Equal(t, "in-progress", mustSnapshot(t, sess2).Issues[0].Status)
}

func TestCommit_OnlyWritesDirtyFiles(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
		"2-b.yaml": issueYAML(2, "b"),
	})

	var writes []string
	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Write: func(path string, data []byte, perm fs.FileMode) error {
			writes = append(writes, path)
			return AtomicWrite(path, data, perm)
		},
		Lock: LockPlanDir,
	}
	repo := New(plansDir, adapters)

	sess, err := repo.Open("p")
	require.NoError(t, err)

	// Only mutate issue #1.
	iss := mustSnapshot(t, sess).Issues[0] // sorted by filename: 1-a, 2-b
	iss.Status = "in-progress"
	require.NoError(t, sess.UpdateIssue(iss))

	require.NoError(t, sess.Commit(testCfg()))
	require.NoError(t, sess.Close())

	// Only one file write: 1-a.yaml. PRD untouched, issue #2 untouched.
	require.Len(t, writes, 1, "only dirty issue should be written, got %v", writes)
	assert.Contains(t, writes[0], "1-a.yaml")
}

func TestCommit_CreateIssueWritesNewFile(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)

	require.NoError(t, sess.CreateIssue(validIssue(2, "brand-new")))
	require.NoError(t, sess.Commit(testCfg()))
	require.NoError(t, sess.Close())

	// File for the new issue must exist on disk with the canonical name.
	_, err = os.Stat(filepath.Join(plansDir, "p", "issues", "2-brand-new.yaml"))
	require.NoError(t, err, "create issue must write {id}-{slug}.yaml")
}

// --- Slug rename ---

func TestCommit_SlugChangeRenamesIssueFile(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-original.yaml": issueYAML(1, "original"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)

	iss := mustSnapshot(t, sess).Issues[0]
	iss.Slug = "renamed"
	require.NoError(t, sess.UpdateIssue(iss))

	require.NoError(t, sess.Commit(testCfg()))
	require.NoError(t, sess.Close())

	// Old filename gone, new filename present, single issue file in dir.
	_, err = os.Stat(filepath.Join(plansDir, "p", "issues", "1-original.yaml"))
	assert.True(t, errors.Is(err, fs.ErrNotExist), "old filename must be removed after slug rename")

	_, err = os.Stat(filepath.Join(plansDir, "p", "issues", "1-renamed.yaml"))
	require.NoError(t, err, "new canonical filename must exist after slug rename")

	entries, err := os.ReadDir(filepath.Join(plansDir, "p", "issues"))
	require.NoError(t, err)
	yamlCount := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".yaml" {
			yamlCount++
		}
	}
	assert.Equal(t, 1, yamlCount, "rename must not leave both files behind")
}

// --- Best-effort rollback ---

func TestCommit_BestEffortRollbackOnInjectedWriteFailure(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
		"2-b.yaml": issueYAML(2, "b"),
	})

	originalA := mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml"))
	originalB := mustReadFile(t, filepath.Join(plansDir, "p", "issues", "2-b.yaml"))
	originalPrd := mustReadFile(t, filepath.Join(plansDir, "p", "prd.yaml"))

	// Fail the third write. With PRD + 2 issues all dirty, the first two succeed
	// and the third fails — exercising rollback over the prior writes.
	var writeCount int
	failingWrite := func(path string, data []byte, perm fs.FileMode) error {
		writeCount++
		if writeCount == 3 {
			return errors.New("injected write failure")
		}
		return AtomicWrite(path, data, perm)
	}

	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Write: failingWrite,
		Lock:  LockPlanDir,
	}
	repo := New(plansDir, adapters)

	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	prd := mustSnapshot(t, sess).PRD
	prd.Updated = "2026-05-03"
	require.NoError(t, sess.UpdatePrd(prd))

	for _, iss := range mustSnapshot(t, sess).Issues {
		iss.Status = "in-progress"
		require.NoError(t, sess.UpdateIssue(iss))
	}

	err = sess.Commit(testCfg())
	require.Error(t, err)

	// Best effort: prior successful writes are restored to original bytes.
	assert.Equal(t, originalPrd, mustReadFile(t, filepath.Join(plansDir, "p", "prd.yaml")), "prd should be rolled back")
	assert.Equal(t, originalA, mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml")), "issue 1 should be rolled back")
	assert.Equal(t, originalB, mustReadFile(t, filepath.Join(plansDir, "p", "issues", "2-b.yaml")), "issue 2 should be rolled back")
}

func TestCommit_RollbackErrorsJoinedWithOriginalError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
		"2-b.yaml": issueYAML(2, "b"),
	})

	// PRD + 2 issues => 3 writes scheduled. The first write succeeds (and
	// queues an undo). The second write fails, triggering rollback. The
	// rollback then invokes the undo, which also fails. The returned error
	// must surface BOTH the originating write failure and the undo failure
	// so a partial rollback isn't silently dropped.
	var writeCount int
	failingWrite := func(path string, data []byte, perm fs.FileMode) error {
		writeCount++
		switch writeCount {
		case 1:
			return AtomicWrite(path, data, perm)
		case 2:
			return errors.New("forward write boom")
		case 3:
			return errors.New("undo write boom")
		}
		return AtomicWrite(path, data, perm)
	}

	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Write: failingWrite,
		Lock:  LockPlanDir,
	}
	repo := New(plansDir, adapters)

	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	prd := mustSnapshot(t, sess).PRD
	prd.Updated = "2026-05-03"
	require.NoError(t, sess.UpdatePrd(prd))
	for _, iss := range mustSnapshot(t, sess).Issues {
		iss.Status = "in-progress"
		require.NoError(t, sess.UpdateIssue(iss))
	}

	err = sess.Commit(testCfg())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward write boom", "originating write error must be surfaced")
	assert.Contains(t, err.Error(), "undo write boom", "undo failure must not be silently dropped")
}

func TestCommit_RollbackRemovesFreshlyCreatedFile(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	// Fail the second write so the first (new file 2-fresh.yaml) succeeds
	// and rollback exercises the os.Remove undo branch for create.
	var writeCount int
	failingWrite := func(path string, data []byte, perm fs.FileMode) error {
		writeCount++
		if writeCount == 2 {
			return errors.New("injected write failure")
		}
		return AtomicWrite(path, data, perm)
	}

	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Write: failingWrite,
		Lock:  LockPlanDir,
	}
	repo := New(plansDir, adapters)

	sess, err := repo.Open("p")
	require.NoError(t, err)
	defer func() { _ = sess.Close() }()

	require.NoError(t, sess.CreateIssue(validIssue(2, "fresh")))
	updated := mustSnapshot(t, sess).Issues[0]
	updated.Status = "in-progress"
	require.NoError(t, sess.UpdateIssue(updated))

	err = sess.Commit(testCfg())
	require.Error(t, err)

	// The freshly-created file must be cleaned up by rollback so the plan
	// dir lands back at its pre-commit state.
	_, err = os.Stat(filepath.Join(plansDir, "p", "issues", "2-fresh.yaml"))
	assert.True(t, errors.Is(err, fs.ErrNotExist), "create rollback must remove the new file")
}

// --- Lock lifetime ---

func TestClose_DiscardsDirtyChanges(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})
	originalIssue := mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml"))

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)

	iss := mustSnapshot(t, sess).Issues[0]
	iss.Status = "in-progress"
	require.NoError(t, sess.UpdateIssue(iss))

	require.NoError(t, sess.Close())

	// File is unchanged: no commit happened.
	assert.Equal(t, originalIssue, mustReadFile(t, filepath.Join(plansDir, "p", "issues", "1-a.yaml")))
}

func TestConcurrentOpen_SerializesThroughLock(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess1, err := repo.Open("p")
	require.NoError(t, err)

	opened := make(chan struct{})
	go func() {
		sess2, err := repo.Open("p")
		if err == nil {
			close(opened)
			_ = sess2.Close()
		}
	}()

	select {
	case <-opened:
		t.Fatal("second Open succeeded while first session held the lock")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, sess1.Close())

	select {
	case <-opened:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("second Open never unblocked after first Close released the lock")
	}
}
