package backend

import (
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
)

func renderTestIssue() *schema.Issue {
	headed := true
	notes := "Watch the footer."
	return &schema.Issue{
		ID:                 7,
		Slug:               "render-body",
		Name:               "Render body",
		Track:              "intent",
		Points:             3,
		TDD:                true,
		Headed:             &headed,
		Outcome:            "Issues carry the full body.",
		Scope:              "A pure renderer.",
		AcceptanceCriteria: []string{"Sections render", "Footer cites source"},
		Steps:              []string{"Write renderer", "Wire backend"},
		UseCases:           []string{"UC-2", "UC-3"},
		Notes:              &notes,
	}
}

func TestRenderIssueBody_AllFields(t *testing.T) {
	body := renderIssueBody(renderTestIssue(), "linear-sync-full-content")

	assert.Contains(t, body, "## Outcome\n\nIssues carry the full body.")
	assert.Contains(t, body, "## Scope\n\nA pure renderer.")
	assert.Contains(t, body, "## Acceptance criteria\n\n- Sections render\n- Footer cites source")
	assert.Contains(t, body, "## Steps\n\n1. Write renderer\n2. Wire backend")
	assert.Contains(t, body, "## Use cases\n\n- UC-2\n- UC-3")
	assert.Contains(t, body, "## Notes\n\nWatch the footer.")

	assert.Contains(t, body, "**Track:** intent")
	assert.Contains(t, body, "**Points:** 3")
	assert.Contains(t, body, "**TDD:** yes")
	assert.Contains(t, body, "**Headed:** yes")
	assert.Contains(t, body, "`.plan-bender/plans/linear-sync-full-content/issues/7-render-body.json`")
	assert.Contains(t, body, "overwritten on the next sync")
}

func TestRenderIssueBody_MissingNotes(t *testing.T) {
	issue := renderTestIssue()
	issue.Notes = nil

	body := renderIssueBody(issue, "linear-sync-full-content")

	assert.NotContains(t, body, "## Notes")
	assert.Contains(t, body, "## Use cases")
}

func TestRenderIssueBody_MissingUseCases(t *testing.T) {
	issue := renderTestIssue()
	issue.UseCases = nil

	body := renderIssueBody(issue, "linear-sync-full-content")

	assert.NotContains(t, body, "## Use cases")
	assert.Contains(t, body, "## Notes")
}

func TestRenderIssueBody_NilHeaded(t *testing.T) {
	issue := renderTestIssue()
	issue.Headed = nil

	body := renderIssueBody(issue, "linear-sync-full-content")

	assert.Contains(t, body, "**Headed:** no")
}

func TestRenderIssueBody_NoEmptyHeadings(t *testing.T) {
	issue := &schema.Issue{
		ID:      1,
		Slug:    "bare",
		Track:   "intent",
		Points:  1,
		Outcome: "Bare outcome.",
		Scope:   "Bare scope.",
	}

	body := renderIssueBody(issue, "demo")

	for _, heading := range []string{"## Acceptance criteria", "## Steps", "## Use cases", "## Notes"} {
		assert.NotContains(t, body, heading)
	}
	assert.False(t, strings.Contains(body, "\n\n\n\n"), "no runs of blank lines")
}
