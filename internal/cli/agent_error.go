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
	ErrConfigError      ErrorCode = "CONFIG_ERROR"
	ErrInternal         ErrorCode = "INTERNAL"
)

// AgentError is an error with a machine-readable code for agent consumers.
type AgentError struct {
	msg  string
	Code ErrorCode
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
}

// writeErrorJSON writes a structured JSON error to w.
// If err is an *AgentError, its code is used; otherwise ErrInternal is used.
func writeErrorJSON(w io.Writer, err error) {
	code := ErrInternal
	var agentErr *AgentError
	if errors.As(err, &agentErr) {
		code = agentErr.Code
	}

	_ = json.NewEncoder(w).Encode(errorJSON{
		Error: err.Error(),
		Code:  string(code),
	})
}
