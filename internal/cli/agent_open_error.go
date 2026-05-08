package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
)

// openErrorToAgent maps a planrepo.Open / loadSnapshot error onto the right
// AgentError. fs.ErrNotExist means the slug isn't on disk; a *planrepo.ParseError
// means the YAML on disk is malformed (user input — INVALID_PLAN); anything
// else is genuinely internal.
//
// INVALID_PLAN exists so callers can distinguish "your plan files don't parse"
// from "the CLI is broken." The colon-in-list YAML footgun is by far the most
// common trigger; the hint nudges editors to single-quote the offending item.
func openErrorToAgent(slug string, err error) *AgentError {
	if errors.Is(err, fs.ErrNotExist) {
		return NewAgentError(fmt.Sprintf("plan %q not found: %s", slug, err), ErrPlanNotFound)
	}
	var parseErr *planrepo.ParseError
	if errors.As(err, &parseErr) {
		ae := NewAgentError(parseErr.Error(), ErrInvalidPlan)
		ae.File = parseErr.File
		ae.Line = parseErr.Line
		ae.Hint = "yaml decode failed; if a list item contains ': ', single-quote the entry — bare 'foo: bar' parses as a map"
		return ae
	}
	return NewAgentError("opening plan: "+err.Error(), ErrInternal)
}
