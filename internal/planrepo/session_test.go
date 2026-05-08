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
		"10-tenth.yaml":     issueYAML(10, "tenth"),
		"2-second.yaml":     issueYAML(2, "second"),
		"1-first.yaml":      issueYAML(1, "first"),
		"20-twentieth.yaml": issueYAML(20, "twentieth"),
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

	var parseErr *ParseError
	require.ErrorAs(t, err, &parseErr, "open should surface yaml decode failures as *ParseError")
	require.Contains(t, parseErr.File, "1-broken.yaml")
}

func TestOpen_ColonInListItem_ReportsLine(t *testing.T) {
	// Reproduces the colon-in-list YAML footgun: a list item containing
	// `: ` mid-prose decodes as a !!map. yaml.v3 reports the line; the
	// loader must propagate it on the ParseError so callers can render
	// file:line.
	plansDir := filepath.Join(t.TempDir(), "plans")
	issue := `id: 1
slug: bad
name: Bad
track: intent
status: todo
priority: high
points: 1
labels: []
blocked_by: []
blocking: []
created: "2026-01-01"
updated: "2026-01-02"
outcome: ok
scope: small
acceptance_criteria: []
steps:
  - first step
  - some/path/file.ts — implement methodName: when X is true do Y
use_cases: []
`
	writePlan(t, plansDir, "bad", validPrd, map[string]string{
		"1-bad.yaml": issue,
	})

	repo := NewProd(plansDir)
	_, err := repo.Open("bad")
	require.Error(t, err)

	var parseErr *ParseError
	require.ErrorAs(t, err, &parseErr)
	require.Contains(t, parseErr.File, "1-bad.yaml")
	require.Greater(t, parseErr.Line, 0, "expected yaml.v3 to report a line; got %q", parseErr.Err)
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
