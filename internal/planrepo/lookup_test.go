package planrepo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenOrCreate_FreshSlugReturnsEmptySnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	repo := NewProd(plansDir)
	sess, err := repo.OpenOrCreate("brand-new")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	snap, err := sess.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "brand-new", snap.Slug)
	assert.Empty(t, snap.PRD.Name)
	assert.Empty(t, snap.Issues)
}

func TestOpenOrCreate_ExistingPlanLoadsSnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.OpenOrCreate("p")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	snap, err := sess.Snapshot()
	require.NoError(t, err)
	assert.Equal(t, "Test Plan", snap.PRD.Name)
	require.Len(t, snap.Issues, 1)
}

func TestOpenOrCreate_HalfBuiltPlanDirReturnsLoadError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "broken", "issues"), 0o755))

	repo := NewProd(plansDir)
	_, err := repo.OpenOrCreate("broken")
	require.Error(t, err, "incomplete plan dir must surface load error")
}

func TestOpenOrCreate_FreshAllowsCommit(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))

	repo := NewProd(plansDir)
	sess, err := repo.OpenOrCreate("fresh")
	require.NoError(t, err)
	defer sess.Close()

	prd := mustValidPRD("fresh")
	require.NoError(t, sess.UpdatePrd(prd))
	require.NoError(t, sess.Commit(testCfg()))

	body, err := os.ReadFile(filepath.Join(plansDir, "fresh", "prd.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "name: Fresh Plan")
}

func TestFindIssueProject_DeterministicSorted(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Write two plans where both contain an issue prefixed "5-": only the
	// alphabetically-first slug should win the lookup.
	writePlan(t, plansDir, "zeta", validPrd, map[string]string{
		"5-z.yaml": issueYAML(5, "z"),
	})
	writePlan(t, plansDir, "alpha", validPrd, map[string]string{
		"5-a.yaml": issueYAML(5, "a"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(5)
	require.NoError(t, err)
	assert.Equal(t, "alpha", slug)
}

func TestFindIssueProject_FindsAcrossPlans(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "first", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})
	writePlan(t, plansDir, "second", validPrd, map[string]string{
		"2-b.yaml": issueYAML(2, "b"),
		"3-c.yaml": issueYAML(3, "c"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(3)
	require.NoError(t, err)
	assert.Equal(t, "second", slug)
}

func TestFindIssueProject_MissingReturnsError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	_, err := repo.FindIssueProject(999)
	require.Error(t, err)
}

func TestFindIssueProject_SkipsHiddenAndArchive(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Create a .archive dir that contains an issue with the same id —
	// it must NOT be returned. Production writes archived plans here and
	// expects findProject to ignore it.
	archivedIssuesDir := filepath.Join(plansDir, ".archive", "old", "issues")
	require.NoError(t, os.MkdirAll(archivedIssuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(archivedIssuesDir, "7-x.yaml"), []byte(issueYAML(7, "x")), 0o644))

	writePlan(t, plansDir, "live", validPrd, map[string]string{
		"7-y.yaml": issueYAML(7, "y"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(7)
	require.NoError(t, err)
	assert.Equal(t, "live", slug)
}
