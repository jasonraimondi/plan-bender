package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrossRefs_ValidRefs(t *testing.T) {
	prd := validPrd()
	prd.UseCases = []UseCase{{ID: "UC-1", Description: "test"}}
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; i.Blocking = []int{2}; i.UseCases = []string{"UC-1"}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{1}; i.Blocking = []int{}; i.UseCases = []string{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues)
	assert.Empty(t, errs)
}

func TestCrossRefs_MissingBlockedByTarget(t *testing.T) {
	prd := validPrd()
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{99}; i.Blocking = []int{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "#99")
}

func TestCrossRefs_MissingBlockingTarget(t *testing.T) {
	prd := validPrd()
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; i.Blocking = []int{99}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "#99")
}

func TestCrossRefs_BrokenSymmetry(t *testing.T) {
	prd := validPrd()
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{2}; i.Blocking = []int{}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{}; i.Blocking = []int{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "does not list")
}

func TestCrossRefs_UnknownUseCase(t *testing.T) {
	prd := validPrd()
	prd.UseCases = []UseCase{{ID: "UC-1", Description: "test"}}
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.UseCases = []string{"UC-99"}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "UC-99")
}
