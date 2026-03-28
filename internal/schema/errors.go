package schema

import (
	"fmt"
	"strings"
)

// ValidationError represents a single field-level validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) String() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// FormatErrors formats a slice of ValidationErrors as a single error string.
func FormatErrors(errs []ValidationError) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.String()
	}
	return strings.Join(msgs, "; ")
}
