package planrepo

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlansValidate_ValidPlan(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "good", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	res := repo.Validate("good", testCfg())

	assert.True(t, res.Valid)
	assert.Empty(t, res.PRD.Errors)
	require.Len(t, res.Issues, 1)
	assert.Empty(t, res.Issues[0].Errors)
}

func TestPlansValidate_MissingPlan_SurfacesAsPrdError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	repo := NewProd(plansDir)
	res := repo.Validate("ghost", testCfg())

	assert.False(t, res.Valid)
	assert.Equal(t, filepath.Join("ghost", "prd.yaml"), res.PRD.File)
	assert.NotEmpty(t, res.PRD.Errors, "open failure must surface as a structured PRD error")
}

func TestPlansValidate_MalformedIssueYAML_AttributedToIssueFile(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "broken", validPrd, map[string]string{
		"1-bad.yaml": "::not yaml::",
	})

	repo := NewProd(plansDir)
	res := repo.Validate("broken", testCfg())

	assert.False(t, res.Valid)
	// PRD parses fine; the parse error is on the issue file, so attribute
	// it there instead of misreporting under prd.yaml.
	assert.Empty(t, res.PRD.Errors)
	require.Len(t, res.Issues, 1)
	assert.Contains(t, res.Issues[0].File, "1-bad.yaml")
	assert.NotEmpty(t, res.Issues[0].Errors)
}

func TestPlansValidate_MalformedPRD_AttributedToPRDFile(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// Write a PRD that fails strict yaml decoding (KnownFields rejects
	// unexpected_field). Write at least one issue so the failure isn't
	// confused with "no plan dir".
	planDir := filepath.Join(plansDir, "broken")
	require.NoError(t, mkdirAll(t, filepath.Join(planDir, "issues")))
	require.NoError(t, writeFile(t, filepath.Join(planDir, "prd.yaml"), "not_a_real_field: x"))
	require.NoError(t, writeFile(t, filepath.Join(planDir, "issues", "1-a.yaml"), issueYAML(1, "a")))

	repo := NewProd(plansDir)
	res := repo.Validate("broken", testCfg())

	assert.False(t, res.Valid)
	assert.NotEmpty(t, res.PRD.Errors)
	assert.Equal(t, filepath.Join("broken", "prd.yaml"), res.PRD.File)
}

func TestPlansValidate_ReleasesLockSoNextOpenSucceeds(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "good", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	_ = repo.Validate("good", testCfg())

	sess, err := repo.Open("good")
	require.NoError(t, err, "Validate must release the lock before returning")
	require.NoError(t, sess.Close())
}
