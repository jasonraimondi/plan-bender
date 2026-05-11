package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
)

// openErrorToAgent maps a planrepo.Open / loadSnapshot error onto the right
// AgentError. fs.ErrNotExist means the slug isn't on disk; a *planrepo.ParseError
// means the JSON on disk is malformed (user input — INVALID_PLAN); anything
// else is genuinely internal.
func openErrorToAgent(slug string, err error) *AgentError {
	if errors.Is(err, fs.ErrNotExist) {
		return NewAgentError(fmt.Sprintf("plan %q not found: %s", slug, err), ErrPlanNotFound)
	}
	var parseErr *planrepo.ParseError
	if errors.As(err, &parseErr) {
		ae := NewAgentError(parseErr.Error(), ErrInvalidPlan)
		ae.File = parseErr.File
		ae.Line = parseErr.Line
		ae.Hint = "json decode failed; check the reported line for a missing quote, comma, or stray trailing comma"
		return ae
	}
	return NewAgentError("opening plan: "+err.Error(), ErrInternal)
}
