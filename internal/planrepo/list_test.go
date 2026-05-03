package planrepo

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_NonexistentDirReturnsEmpty(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans-does-not-exist")

	repo := NewProd(plansDir)
	plans, err := repo.List()
	require.NoError(t, err)
	assert.Empty(t, plans)
}

func TestList_EmptyDir(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, mkdirAll(t, plansDir))

	repo := NewProd(plansDir)
	plans, err := repo.List()
	require.NoError(t, err)
	assert.Empty(t, plans)
}

func TestList_ReturnsValidPlans(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	prdAlpha := `name: Plan Alpha
slug: alpha
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: A
why: A
outcome: A
`
	prdBeta := `name: Plan Beta
slug: beta
status: draft
created: "2026-01-01"
updated: "2026-01-02"
description: B
why: B
outcome: B
`
	writePlan(t, plansDir, "alpha", prdAlpha, map[string]string{
		"1-one.yaml": issueYAML(1, "one"),
	})
	writePlan(t, plansDir, "beta", prdBeta, nil)

	repo := NewProd(plansDir)
	plans, err := repo.List()
	require.NoError(t, err)
	require.Len(t, plans, 2)

	bySlug := map[string]PlanSummary{}
	for _, p := range plans {
		bySlug[p.Slug] = p
	}
	assert.Equal(t, "Plan Alpha", bySlug["alpha"].Name)
	assert.Equal(t, "active", bySlug["alpha"].Status)
	assert.Equal(t, 1, bySlug["alpha"].Issues)
	assert.Equal(t, "Plan Beta", bySlug["beta"].Name)
	assert.Equal(t, "draft", bySlug["beta"].Status)
	assert.Equal(t, 0, bySlug["beta"].Issues)
}

func TestList_SkipsMalformedPrdAndDoesNotFailWholeListing(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	// Good plan.
	writePlan(t, plansDir, "good", `name: Good
slug: good
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: G
why: G
outcome: G
`, map[string]string{
		"1-only.yaml": issueYAML(1, "only"),
	})

	// Plan with malformed PRD (invalid YAML).
	writePlan(t, plansDir, "broken-prd", "::not yaml::", nil)

	// Plan with malformed issue file.
	writePlan(t, plansDir, "broken-issue", `name: Broken Issue
slug: broken-issue
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: X
why: X
outcome: X
`, map[string]string{
		"1-broken.yaml": "::not yaml::",
	})

	repo := NewProd(plansDir)
	plans, err := repo.List()
	require.NoError(t, err)

	slugs := []string{}
	for _, p := range plans {
		slugs = append(slugs, p.Slug)
	}
	assert.Contains(t, slugs, "good")
	assert.NotContains(t, slugs, "broken-prd", "malformed PRD plans should be skipped")
	assert.NotContains(t, slugs, "broken-issue", "malformed issue plans should be skipped")
}

func TestList_SkipsHiddenAndNonDirEntries(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	require.NoError(t, mkdirAll(t, plansDir))
	require.NoError(t, writeFile(t, filepath.Join(plansDir, ".hidden"), "ignore me"))
	require.NoError(t, writeFile(t, filepath.Join(plansDir, "stray.txt"), "not a plan"))
	writePlan(t, plansDir, "real", validPrd, nil)

	repo := NewProd(plansDir)
	plans, err := repo.List()
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "real", plans[0].Slug)
}
