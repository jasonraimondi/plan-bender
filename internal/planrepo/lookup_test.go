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

	snap := sess.Snapshot()
	require.NotNil(t, snap)
	assert.Equal(t, "brand-new", snap.Slug)
	assert.Empty(t, snap.PRD.Name)
	assert.Empty(t, snap.Issues)
}

func TestOpenOrCreate_ExistingPlanLoadsSnapshot(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.json": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	sess, err := repo.OpenOrCreate("p")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	assert.Equal(t, "Test Plan", sess.Snapshot().PRD.Name)
	require.Len(t, sess.Snapshot().Issues, 1)
}

func TestOpenOrCreate_HalfBuiltPlanDirReturnsLoadError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "broken", "issues"), 0o755))

	repo := NewProd(plansDir)
	_, err := repo.OpenOrCreate("broken")
	require.Error(t, err, "incomplete plan dir must surface load error")
}

// TestOpenOrCreate_LegacyYAMLHintsAtMigrate ensures the loader nudges users
// who upgraded the binary without running `pb migrate` toward the fix instead
// of returning a bare "prd.json does not exist" error.
func TestOpenOrCreate_LegacyYAMLHintsAtMigrate(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "demo", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "demo", "prd.yaml"),
		[]byte("name: Demo\nslug: demo\n"), 0o644))

	repo := NewProd(plansDir)
	_, err := repo.OpenOrCreate("demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pb migrate")
	assert.Contains(t, err.Error(), "prd.yaml")
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

	body, err := os.ReadFile(filepath.Join(plansDir, "fresh", "prd.json"))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"name": "Fresh Plan"`)
}

func TestFindIssueProject_DeterministicSorted(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Write two plans where both contain an issue prefixed "5-": only the
	// alphabetically-first slug should win the lookup.
	writePlan(t, plansDir, "zeta", validPrd, map[string]string{
		"5-z.json": issueYAML(5, "z"),
	})
	writePlan(t, plansDir, "alpha", validPrd, map[string]string{
		"5-a.json": issueYAML(5, "a"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(5)
	require.NoError(t, err)
	assert.Equal(t, "alpha", slug)
}

func TestFindIssueProject_FindsAcrossPlans(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "first", validPrd, map[string]string{
		"1-a.json": issueYAML(1, "a"),
	})
	writePlan(t, plansDir, "second", validPrd, map[string]string{
		"2-b.json": issueYAML(2, "b"),
		"3-c.json": issueYAML(3, "c"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(3)
	require.NoError(t, err)
	assert.Equal(t, "second", slug)
}

func TestFindIssueProject_MissingReturnsError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.json": issueYAML(1, "a"),
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
	require.NoError(t, os.WriteFile(filepath.Join(archivedIssuesDir, "7-x.json"), []byte(issueYAML(7, "x")), 0o644))

	writePlan(t, plansDir, "live", validPrd, map[string]string{
		"7-y.json": issueYAML(7, "y"),
	})

	repo := NewProd(plansDir)
	slug, err := repo.FindIssueProject(7)
	require.NoError(t, err)
	assert.Equal(t, "live", slug)
}
