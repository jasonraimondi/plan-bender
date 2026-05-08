package cli

import (
	"encoding/json"
	"errors"
	"io"
)

// ErrorCode identifies the category of agent error.
type ErrorCode string

const (
	ErrPlanNotFound     ErrorCode = "PLAN_NOT_FOUND"
	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrInvalidPlan      ErrorCode = "INVALID_PLAN"
	ErrConfigError      ErrorCode = "CONFIG_ERROR"
	ErrInternal         ErrorCode = "INTERNAL"
)

// AgentError is an error with a machine-readable code for agent consumers.
// File, Line, and Hint are optional structured context surfaced to JSON
// callers (e.g. for INVALID_PLAN, the offending file path and decoder line).
type AgentError struct {
	msg  string
	Code ErrorCode
	File string
	Line int
	Hint string
}

func NewAgentError(msg string, code ErrorCode) *AgentError {
	return &AgentError{msg: msg, Code: code}
}

func (e *AgentError) Error() string {
	return e.msg
}

// errorJSON is the wire format for agent error responses.
type errorJSON struct {
	Error string `json:"error"`
	Code  string `json:"code"`
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
	Hint  string `json:"hint,omitempty"`
}

// writeErrorJSON writes a structured JSON error to w.
// If err is an *AgentError, its code (and optional file/line/hint) are used;
// otherwise ErrInternal is used.
func writeErrorJSON(w io.Writer, err error) {
	out := errorJSON{Error: err.Error(), Code: string(ErrInternal)}
	var agentErr *AgentError
	if errors.As(err, &agentErr) {
		out.Code = string(agentErr.Code)
		out.File = agentErr.File
		out.Line = agentErr.Line
		out.Hint = agentErr.Hint
	}
	_ = json.NewEncoder(w).Encode(out)
}
