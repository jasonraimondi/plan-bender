package schema

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	return data
}

func TestValidatePlan_ValidPlan(t *testing.T) {
	prd := validPrd()
	issue := validIssue()

	fsys := fstest.MapFS{
		"test/prd.yaml":                &fstest.MapFile{Data: mustMarshal(t, &prd)},
		"test/issues/1-test-issue.yaml": &fstest.MapFile{Data: mustMarshal(t, &issue)},
	}

	result := ValidatePlan("test", config.Defaults(), fsys)
	assert.True(t, result.Valid)
	assert.Empty(t, result.PRD.Errors)
	assert.Empty(t, result.CrossRef)
	assert.Empty(t, result.Cycles)
}

func TestValidatePlan_PRDErrors(t *testing.T) {
	badPrd := PrdYaml{Slug: "bad"}

	fsys := fstest.MapFS{
		"test/prd.yaml":   &fstest.MapFile{Data: mustMarshal(t, &badPrd)},
		"test/issues/":    &fstest.MapFile{Mode: 0o755 | fs.ModeDir},
	}

	result := ValidatePlan("test", config.Defaults(), fsys)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.PRD.Errors)
}

func TestValidatePlan_IssueErrors(t *testing.T) {
	prd := validPrd()
	badIssue := validIssue()
	badIssue.Points = 99

	fsys := fstest.MapFS{
		"test/prd.yaml":                &fstest.MapFile{Data: mustMarshal(t, &prd)},
		"test/issues/1-test-issue.yaml": &fstest.MapFile{Data: mustMarshal(t, &badIssue)},
	}

	result := ValidatePlan("test", config.Defaults(), fsys)
	assert.False(t, result.Valid)
}

func TestValidatePlan_DetectsCycle(t *testing.T) {
	prd := validPrd()
	issue1 := validIssue()
	issue1.ID = 1
	issue1.BlockedBy = []int{2}
	issue2 := validIssue()
	issue2.ID = 2
	issue2.Slug = "issue-2"
	issue2.BlockedBy = []int{1}

	fsys := fstest.MapFS{
		"test/prd.yaml":               &fstest.MapFile{Data: mustMarshal(t, &prd)},
		"test/issues/1-test-issue.yaml": &fstest.MapFile{Data: mustMarshal(t, &issue1)},
		"test/issues/2-issue-2.yaml":    &fstest.MapFile{Data: mustMarshal(t, &issue2)},
	}

	result := ValidatePlan("test", config.Defaults(), fsys)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Cycles)
}
