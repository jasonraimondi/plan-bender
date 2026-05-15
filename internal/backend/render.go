package backend

import (
	"fmt"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// renderIssueBody renders an issue into the markdown description published to
// Linear. The plan slug is needed only for the footer's Source path; it is
// not stored on the issue. Optional sections with no content are omitted so
// the body never carries an empty heading.
func renderIssueBody(issue *schema.Issue, slug string) string {
	var b strings.Builder

	section(&b, "Outcome", issue.Outcome)
	section(&b, "Scope", issue.Scope)
	listSection(&b, "Acceptance criteria", issue.AcceptanceCriteria, false)
	listSection(&b, "Steps", issue.Steps, true)
	listSection(&b, "Use cases", issue.UseCases, false)
	if issue.Notes != nil && *issue.Notes != "" {
		section(&b, "Notes", *issue.Notes)
	}

	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "**Track:** %s · **Points:** %d · **TDD:** %s · **Headed:** %s\n\n",
		issue.Track, issue.Points, yesNo(issue.TDD), yesNo(issue.Headed != nil && *issue.Headed))
	fmt.Fprintf(&b, "_Source: `.plan-bender/plans/%s/issues/%d-%s.json`. Linear is published from this file by plan-bender; edits made here are overwritten on the next sync._\n",
		slug, issue.ID, issue.Slug)

	return b.String()
}

// renderProjectBody renders a PRD into the two fields published to a Linear
// project: the short description (the PRD's own description field) and the
// markdown content body carrying the remaining PRD fields as headed sections.
// The content body uses the same footer and overwrite disclaimer as the issue
// body. Optional sections with no content are omitted.
func renderProjectBody(prd *schema.PRD) (description, content string) {
	var b strings.Builder

	section(&b, "Why", prd.Why)
	section(&b, "Outcome", prd.Outcome)
	listSection(&b, "In scope", prd.InScope, false)
	listSection(&b, "Out of scope", prd.OutOfScope, false)
	useCaseSection(&b, prd.UseCases)
	listSection(&b, "Decisions", prd.Decisions, false)
	listSection(&b, "Open questions", prd.OpenQuestions, false)
	listSection(&b, "Risks", prd.Risks, false)
	listSection(&b, "Validation", prd.Validation, false)
	if prd.Notes != nil && *prd.Notes != "" {
		section(&b, "Notes", *prd.Notes)
	}

	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "**Status:** %s\n\n", prd.Status)
	fmt.Fprintf(&b, "_Source: `.plan-bender/plans/%s/prd.json`. Linear is published from this file by plan-bender; edits made here are overwritten on the next sync._\n",
		prd.Slug)

	return prd.Description, b.String()
}

// useCaseSection writes a heading followed by a bulleted list of use cases,
// each rendered as "**id**: description". An empty slice omits the heading.
func useCaseSection(b *strings.Builder, useCases []schema.UseCase) {
	if len(useCases) == 0 {
		return
	}
	b.WriteString("## Use cases\n\n")
	for _, uc := range useCases {
		fmt.Fprintf(b, "- **%s**: %s\n", uc.ID, uc.Description)
	}
	b.WriteString("\n")
}

// section writes a heading and a prose body. Outcome and Scope are required by
// schema validation, so this is only called with non-empty text.
func section(b *strings.Builder, heading, body string) {
	fmt.Fprintf(b, "## %s\n\n%s\n\n", heading, body)
}

// listSection writes a heading followed by an ordered (numbered) or unordered
// (bulleted) list. An empty list omits the heading entirely.
func listSection(b *strings.Builder, heading string, items []string, ordered bool) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", heading)
	for i, item := range items {
		if ordered {
			fmt.Fprintf(b, "%d. %s\n", i+1, item)
		} else {
			fmt.Fprintf(b, "- %s\n", item)
		}
	}
	b.WriteString("\n")
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
