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
	res, err := repo.Validate("good", testCfg())

	require.NoError(t, err)
	assert.True(t, res.Valid)
	assert.Empty(t, res.PRD.Errors)
	require.Len(t, res.Issues, 1)
	assert.Empty(t, res.Issues[0].Errors)
}

func TestPlansValidate_MissingPlan_ReturnsError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")

	repo := NewProd(plansDir)
	_, err := repo.Validate("ghost", testCfg())

	require.Error(t, err, "missing plan must surface as the returned error, not a fake PRD validation error")
}

func TestPlansValidate_MalformedYAML_ReturnsError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "broken", validPrd, map[string]string{
		"1-bad.yaml": "::not yaml::",
	})

	repo := NewProd(plansDir)
	_, err := repo.Validate("broken", testCfg())

	require.Error(t, err, "malformed issue YAML must surface as a load error, not a fake PRD validation error")
}

func TestPlansValidate_ReleasesLockSoNextOpenSucceeds(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "good", validPrd, map[string]string{
		"1-a.yaml": issueYAML(1, "a"),
	})

	repo := NewProd(plansDir)
	_, err := repo.Validate("good", testCfg())
	require.NoError(t, err)

	sess, err := repo.Open("good")
	require.NoError(t, err, "Validate must release the lock before returning")
	require.NoError(t, sess.Close())
}
