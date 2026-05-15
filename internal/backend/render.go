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
