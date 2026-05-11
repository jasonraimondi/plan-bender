package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrossRefs_ValidRefs(t *testing.T) {
	prd := validPrd()
	prd.UseCases = []UseCase{{ID: "UC-1", Description: "test"}}
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; i.Blocking = []int{2}; i.UseCases = []string{"UC-1"}; return i }(),
		func() Issue { i := validIssue(); i.ID = 2; i.BlockedBy = []int{1}; i.Blocking = []int{}; i.UseCases = []string{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefStrict)
	assert.Empty(t, errs)
}

func TestCrossRefs_MissingBlockedByTarget(t *testing.T) {
	prd := validPrd()
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{99}; i.Blocking = []int{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefStrict)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "#99")
}

func TestCrossRefs_MissingBlockingTarget(t *testing.T) {
	prd := validPrd()
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; i.Blocking = []int{99}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefStrict)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "#99")
}

func TestCrossRefs_BrokenSymmetry(t *testing.T) {
	prd := validPrd()
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{2}; i.Blocking = []int{}; return i }(),
		func() Issue { i := validIssue(); i.ID = 2; i.BlockedBy = []int{}; i.Blocking = []int{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefStrict)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "does not list")
}

func TestCrossRefs_UnknownUseCase(t *testing.T) {
	prd := validPrd()
	prd.UseCases = []UseCase{{ID: "UC-1", Description: "test"}}
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.UseCases = []string{"UC-99"}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefStrict)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "UC-99")
}

// Lax mode is the bootstrap path: writing the first issue of a plan with
// forward refs to issues that have not yet been written must not error.
// Symmetry is only checked between issues that both exist in the snapshot.
func TestCrossRefs_Lax_MissingTargetsAccepted(t *testing.T) {
	prd := validPrd()
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{99}; i.Blocking = []int{2, 3}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefLax)
	assert.Empty(t, errs)
}

// Lax mode still flags broken symmetry between two issues that both exist —
// otherwise a plan could end up with mismatched edges that survive every
// per-write commit.
func TestCrossRefs_Lax_StillFlagsBrokenSymmetryBetweenExistingIssues(t *testing.T) {
	prd := validPrd()
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.BlockedBy = []int{2}; i.Blocking = []int{}; return i }(),
		func() Issue { i := validIssue(); i.ID = 2; i.BlockedBy = []int{}; i.Blocking = []int{}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefLax)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "does not list")
}

// Use case validation is mode-independent: the PRD is always written before
// any issue, so unknown use_cases are always an error.
func TestCrossRefs_Lax_StillFlagsUnknownUseCase(t *testing.T) {
	prd := validPrd()
	prd.UseCases = []UseCase{{ID: "UC-1", Description: "test"}}
	issues := []Issue{
		func() Issue { i := validIssue(); i.ID = 1; i.UseCases = []string{"UC-99"}; return i }(),
	}
	errs := ValidateCrossRefs(&prd, issues, CrossRefLax)
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "UC-99")
}
