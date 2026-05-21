package planrepo

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_LoadsSnapshotWithDeterministicIssueOrder(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	// Write issues in non-sorted insertion order with filenames that exercise
	// numeric vs lexicographic sort. We expect lexicographic sort by filename.
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"10-tenth.json":     issueJSON(10, "tenth"),
		"2-second.json":     issueJSON(2, "second"),
		"1-first.json":      issueJSON(1, "first"),
		"20-twentieth.json": issueJSON(20, "twentieth"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.Open("test-plan")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	snap := sess.Snapshot()
	require.NotNil(t, snap)
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
	// Plan directory exists but has no prd.json.
	require.NoError(t, mkdirAll(t, filepath.Join(plansDir, "broken", "issues")))

	repo := NewProd(plansDir)
	_, err := repo.Open("broken")
	require.Error(t, err)
}

func TestOpen_MalformedIssueErrors(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-broken.json": "::not json::",
	})

	repo := NewProd(plansDir)
	_, err := repo.Open("test-plan")
	require.Error(t, err)

	var parseErr *ParseError
	require.ErrorAs(t, err, &parseErr, "open should surface json decode failures as *ParseError")
	require.Contains(t, parseErr.File, "1-broken.json")
}

// A plan with a PRD but no issues/ dir yet (a freshly written PRD, before
// decomposition) is valid: a missing issues dir means zero issues, not a
// contract violation. Open must load it with an empty issue set.
func TestOpen_NoIssuesDir(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	planDir := filepath.Join(plansDir, "no-issues")
	require.NoError(t, mkdirAll(t, planDir))
	require.NoError(t, writeFile(t, filepath.Join(planDir, "prd.json"), validPrd))

	repo := NewProd(plansDir)
	sess, err := repo.Open("no-issues")
	require.NoError(t, err)
	defer sess.Close()
	assert.Empty(t, sess.Snapshot().Issues)
}

func TestClose_Idempotent(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-first.json": issueJSON(1, "first"),
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
		"1-first.json": issueJSON(1, "first"),
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

func TestOpen_FailedLoadReleasesLock(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Plan exists but issue is malformed: load fails after lock is taken.
	writePlan(t, plansDir, "test-plan", validPrd, map[string]string{
		"1-broken.json": "::not json::",
	})

	repo := NewProd(plansDir)
	_, err := repo.Open("test-plan")
	require.Error(t, err)

	// A subsequent Open of an unrelated, valid plan must not block on a
	// leaked lock. Use a fresh slug.
	writePlan(t, plansDir, "good", validPrd, map[string]string{
		"1-first.json": issueJSON(1, "first"),
	})
	sess, err := repo.Open("good")
	require.NoError(t, err)
	require.NoError(t, sess.Close())
}
