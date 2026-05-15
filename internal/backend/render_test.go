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

func renderTestPRD() *schema.PRD {
	notes := "Mind the footer."
	return &schema.PRD{
		Name:        "Linear sync full content",
		Slug:        "linear-sync-full-content",
		Status:      "active",
		Created:     "2026-05-15",
		Updated:     "2026-05-15",
		Description: "Publish the full PRD to Linear.",
		Why:         "Linear projects were frozen at creation.",
		Outcome:     "Linear carries the full PRD.",
		InScope:     []string{"Project renderer", "projectUpdate mutation"},
		OutOfScope:  []string{"Two-way sync"},
		UseCases: []schema.UseCase{
			{ID: "UC-1", Description: "Author pushes a plan"},
			{ID: "UC-3", Description: "Author re-pushes a plan"},
		},
		Decisions:     []string{"One-way publish"},
		OpenQuestions: []string{"Rate limits?"},
		Risks:         []string{"Overwrites manual edits"},
		Validation:    []string{"Table tests"},
		Notes:         &notes,
	}
}

func TestRenderProjectBody_AllFields(t *testing.T) {
	description, content := renderProjectBody(renderTestPRD())

	assert.Equal(t, "Publish the full PRD to Linear.", description)

	assert.Contains(t, content, "## Why\n\nLinear projects were frozen at creation.")
	assert.Contains(t, content, "## Outcome\n\nLinear carries the full PRD.")
	assert.Contains(t, content, "## In scope\n\n- Project renderer\n- projectUpdate mutation")
	assert.Contains(t, content, "## Out of scope\n\n- Two-way sync")
	assert.Contains(t, content, "## Use cases\n\n- **UC-1**: Author pushes a plan\n- **UC-3**: Author re-pushes a plan")
	assert.Contains(t, content, "## Decisions\n\n- One-way publish")
	assert.Contains(t, content, "## Open questions\n\n- Rate limits?")
	assert.Contains(t, content, "## Risks\n\n- Overwrites manual edits")
	assert.Contains(t, content, "## Validation\n\n- Table tests")
	assert.Contains(t, content, "## Notes\n\nMind the footer.")

	assert.Contains(t, content, "**Status:** active")
	assert.Contains(t, content, "`.plan-bender/plans/linear-sync-full-content/prd.json`")
	assert.Contains(t, content, "overwritten on the next sync")
}

func TestRenderProjectBody_MissingNotes(t *testing.T) {
	prd := renderTestPRD()
	prd.Notes = nil

	_, content := renderProjectBody(prd)

	assert.NotContains(t, content, "## Notes")
	assert.Contains(t, content, "## Validation")
}

func TestRenderProjectBody_NoEmptyHeadings(t *testing.T) {
	prd := &schema.PRD{
		Name:        "Bare",
		Slug:        "bare",
		Status:      "draft",
		Created:     "2026-05-15",
		Updated:     "2026-05-15",
		Description: "Bare description.",
		Why:         "Bare why.",
		Outcome:     "Bare outcome.",
	}

	_, content := renderProjectBody(prd)

	for _, heading := range []string{
		"## In scope", "## Out of scope", "## Use cases", "## Decisions",
		"## Open questions", "## Risks", "## Validation", "## Notes",
	} {
		assert.NotContains(t, content, heading)
	}
	assert.False(t, strings.Contains(content, "\n\n\n\n"), "no runs of blank lines")
}
