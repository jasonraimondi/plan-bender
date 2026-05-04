package planrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_LoadsSnapshotWithDeterministicIssueOrder(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	// Write issues in non-sorted insertion order with filenames that exercise
	// numeric vs lexicographic sort. We expect lexicographic sort by filename.
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"10-tenth.yaml":     issueYAML(10, "tenth"),
		"2-second.yaml":     issueYAML(2, "second"),
		"1-first.yaml":      issueYAML(1, "first"),
		"20-twentieth.yaml": issueYAML(20, "twentieth"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("test-plan")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	snap, err := sess.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "test-plan", snap.Slug)
	assert.Equal(t, "Test Plan", snap.PRD.Name)

	// Lexicographic sort by filename: 1-, 10-, 2-, 20-.
	require.Len(t, snap.Issues, 4)
	assert.Equal(t, 1, snap.Issues[0].ID)
	assert.Equal(t, 10, snap.Issues[1].ID)
	assert.Equal(t, 2, snap.Issues[2].ID)
	assert.Equal(t, 20, snap.Issues[3].ID)
}

func TestOpen_PlanDirMissing(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	repo := NewProd(plansDir)

	_, err := repo.Open("does-not-exist")
	require.Error(t, err)
}

func TestOpen_PrdMissing(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Plan directory exists but has no prd.yaml.
	require.NoError(t, mkdirAll(t, filepath.Join(plansDir, "broken", "issues")))

	repo := NewProd(plansDir)
	_, err := repo.Open("broken")
	require.Error(t, err)
}

func TestOpen_MalformedIssueErrors(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-broken.yaml": "::not yaml::",
	})

	repo := NewProd(plansDir)
	_, err := repo.Open("test-plan")
	require.Error(t, err)
}

func TestOpen_NoIssuesDir(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Plan directory with PRD but no issues dir at all.
	planDir := filepath.Join(plansDir, "no-issues")
	require.NoError(t, mkdirAll(t, planDir))
	require.NoError(t, writeFile(t, filepath.Join(planDir, "prd.yaml"), validPrd))

	repo := NewProd(plansDir)
	_, err := repo.Open("no-issues")
	require.Error(t, err, "missing issues dir is a contract violation")
}

func TestClose_Idempotent(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-first.yaml": issueYAML(1, "first"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("test-plan")
	require.NoError(t, err)

	require.NoError(t, sess.Close())
	require.NoError(t, sess.Close(), "second Close must not error")
}

func TestClose_ReleasesLockSoNextOpenSucceeds(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-first.yaml": issueYAML(1, "first"),
	})

	repo := NewProd(plansDir)

	sess1, err := repo.Open("test-plan")
	require.NoError(t, err)
	require.NoError(t, sess1.Close())

	// If lock was not released, this would block forever (POSIX flock is
	// per-process exclusive). Within the same process, reentry would also
	// succeed, so this is a smoke check that Close at least executes the
	// release. The session lock test below covers cross-process semantics.
	sess2, err := repo.Open("test-plan")
	require.NoError(t, err)
	require.NoError(t, sess2.Close())
}

func TestSnapshot_DefensiveCopyOfIssues(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-first.yaml":  issueYAML(1, "first"),
		"2-second.yaml": issueYAML(2, "second"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("test-plan")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	// Seed in-session state with non-empty inner slices and a pointer field
	// so the deep-copy contract has something to alias if it regresses.
	seed := mustSnapshot(t, sess).Issues[0]
	seed.Labels = []string{"AFK"}
	seed.BlockedBy = []int{2}
	branch := "feature/initial"
	seed.Branch = &branch
	require.NoError(t, sess.UpdateIssue(seed))

	first, err := sess.Snapshot()
	require.NoError(t, err)
	require.Len(t, first.Issues, 2)

	// Outer slice tampering.
	first.Issues[0].Slug = "TAMPERED"
	first.Issues = append(first.Issues, schema.IssueYaml{ID: 999})

	// Inner slice tampering.
	first.Issues[0].Labels[0] = "TAMPERED-LABEL"
	first.Issues[0].BlockedBy[0] = 999

	// Pointer field tampering.
	*first.Issues[0].Branch = "TAMPERED-BRANCH"

	second, err := sess.Snapshot()
	require.NoError(t, err)
	require.Len(t, second.Issues, 2, "appended issue must not appear in next Snapshot")
	assert.Equal(t, "first", second.Issues[0].Slug, "slug mutation must not bleed into session")
	assert.Equal(t, []string{"AFK"}, second.Issues[0].Labels, "inner slice mutation must not bleed into session")
	assert.Equal(t, []int{2}, second.Issues[0].BlockedBy, "inner int slice mutation must not bleed into session")
	require.NotNil(t, second.Issues[0].Branch)
	assert.Equal(t, "feature/initial", *second.Issues[0].Branch, "pointer-field mutation must not bleed into session")
}

func TestSession_AfterCloseRejectsAllOps(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("p")
	require.NoError(t, err)
	require.NoError(t, sess.Close())

	t.Run("Snapshot", func(t *testing.T) {
		_, err := sess.Snapshot()
		require.ErrorIs(t, err, ErrSessionClosed)
	})
	t.Run("UpdatePrd", func(t *testing.T) {
		err := sess.UpdatePrd(schema.PrdYaml{Slug: "p"})
		require.ErrorIs(t, err, ErrSessionClosed)
	})
	t.Run("UpdateIssue", func(t *testing.T) {
		err := sess.UpdateIssue(schema.IssueYaml{ID: 1})
		require.ErrorIs(t, err, ErrSessionClosed)
	})
	t.Run("CreateIssue", func(t *testing.T) {
		err := sess.CreateIssue(schema.IssueYaml{ID: 2})
		require.ErrorIs(t, err, ErrSessionClosed)
	})
	t.Run("Commit", func(t *testing.T) {
		err := sess.Commit(testCfg())
		require.ErrorIs(t, err, ErrSessionClosed)
	})
}

func TestClose_PropagatesReleaseError(t *testing.T) {
	wantErr := errors.New("unlock blew up")
	plansDir := t.TempDir()
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})
	repo := New(plansDir, Adapters{
		FS:     os.DirFS(plansDir),
		Write:  func(_ string, _ []byte, _ fs.FileMode) error { return nil },
		Mkdir:  func(_ string, _ fs.FileMode) error { return nil },
		Remove: prodRemove,
		Lock: func(_ string) (func() error, error) {
			return func() error { return wantErr }, nil
		},
	})
	sess, err := repo.Open("p")
	require.NoError(t, err)

	closeErr := sess.Close()
	require.ErrorIs(t, closeErr, wantErr)

	// Idempotent: second Close returns the same error without re-running release.
	closeErr2 := sess.Close()
	require.ErrorIs(t, closeErr2, wantErr)
}

func TestOpen_FailedLoadReleasesLock(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Plan exists but issue is malformed: load fails after lock is taken.
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-broken.yaml": "::not yaml::",
	})

	repo := NewProd(plansDir)
	_, err := repo.Open("test-plan")
	require.Error(t, err)

	// A subsequent Open of an unrelated, valid plan must not block on a
	// leaked lock. Use a fresh slug.
	writePlan(t, plansDir, "good", validPrd, map[string]string{
		"1-first.yaml": issueYAML(1, "first"),
	})
	sess, err := repo.Open("good")
	require.NoError(t, err)
	require.NoError(t, sess.Close())
}
