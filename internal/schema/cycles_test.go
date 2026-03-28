package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectCycles_NoCycle(t *testing.T) {
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{1}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 3; i.BlockedBy = []int{2}; return i }(),
	}
	errs := DetectCycles(issues)
	assert.Empty(t, errs)
}

func TestDetectCycles_SimpleCycle(t *testing.T) {
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{2}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{1}; return i }(),
	}
	errs := DetectCycles(issues)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0], "#1")
	assert.Contains(t, errs[0], "#2")
}

func TestDetectCycles_TransitiveCycle(t *testing.T) {
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{3}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{1}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 3; i.BlockedBy = []int{2}; return i }(),
	}
	errs := DetectCycles(issues)
	assert.Len(t, errs, 1)
}

func TestDetectCycles_DisconnectedGraph(t *testing.T) {
	issues := []IssueYaml{
		func() IssueYaml { i := validIssue(); i.ID = 1; i.BlockedBy = []int{}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 2; i.BlockedBy = []int{}; return i }(),
		func() IssueYaml { i := validIssue(); i.ID = 3; i.BlockedBy = []int{}; return i }(),
	}
	errs := DetectCycles(issues)
	assert.Empty(t, errs)
}
