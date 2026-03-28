package schema

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func defaultConfig() config.Config {
	return config.Defaults()
}

func validIssue() IssueYaml {
	return IssueYaml{
		ID:                 1,
		Slug:               "test-issue",
		Name:               "Test issue",
		Track:              "intent",
		Status:             "backlog",
		Priority:           "high",
		Points:             2,
		Labels:             []string{"AFK"},
		Assignee:           nil,
		BlockedBy:          []int{},
		Blocking:           []int{},
		Branch:             nil,
		PR:                 nil,
		LinearID:           nil,
		Created:            "2026-03-26",
		Updated:            "2026-03-26",
		TDD:                false,
		Outcome:            "Something useful",
		Scope:              "Do the thing",
		AcceptanceCriteria: []string{"It works"},
		Steps:              []string{"Step 1"},
		UseCases:           []string{},
	}
}

func TestIssueValidate_Valid(t *testing.T) {
	issue := validIssue()
	errs := issue.Validate(defaultConfig())
	assert.Empty(t, errs)
}

func TestIssueValidate_InvalidTrack(t *testing.T) {
	issue := validIssue()
	issue.Track = "nope"
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "track")
}

func TestIssueValidate_InvalidStatus(t *testing.T) {
	issue := validIssue()
	issue.Status = "invalid"
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "status")
}

func TestIssueValidate_InvalidPriority(t *testing.T) {
	issue := validIssue()
	issue.Priority = "critical"
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "priority")
}

func TestIssueValidate_PointsAboveMax(t *testing.T) {
	issue := validIssue()
	issue.Points = 99
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "points")
}

func TestIssueValidate_ZeroPoints(t *testing.T) {
	issue := validIssue()
	issue.Points = 0
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "points")
}

func TestIssueValidate_SelfReferenceBlockedBy(t *testing.T) {
	issue := validIssue()
	issue.BlockedBy = []int{1}
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "blocked_by")
}

func TestIssueValidate_SelfReferenceBlocking(t *testing.T) {
	issue := validIssue()
	issue.Blocking = []int{1}
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "blocking")
}

func TestIssueValidate_DuplicateBlockedBy(t *testing.T) {
	issue := validIssue()
	issue.BlockedBy = []int{2, 2}
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "blocked_by")
}

func TestIssueValidate_DuplicateBlocking(t *testing.T) {
	issue := validIssue()
	issue.Blocking = []int{2, 2}
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "blocking")
}

func TestIssueValidate_MissingRequiredFields(t *testing.T) {
	issue := IssueYaml{ID: 1}
	errs := issue.Validate(defaultConfig())
	assertHasFieldError(t, errs, "slug")
	assertHasFieldError(t, errs, "name")
	assertHasFieldError(t, errs, "outcome")
	assertHasFieldError(t, errs, "scope")
}

func TestIssueValidate_AllValidPriorities(t *testing.T) {
	for _, p := range []string{"urgent", "high", "medium", "low"} {
		issue := validIssue()
		issue.Priority = p
		errs := issue.Validate(defaultConfig())
		assert.Empty(t, errs, "priority %q should be valid", p)
	}
}

func TestIssueYaml_RoundTrip(t *testing.T) {
	issue := validIssue()
	data, err := yaml.Marshal(&issue)
	require.NoError(t, err)

	var parsed IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &parsed))
	assert.Equal(t, issue, parsed)
}
