package config

import (
	"fmt"
	"strings"
)

// ConfigError represents a configuration validation failure.
type ConfigError struct {
	Errors []FieldError
}

// FieldError is a single field-level validation error.
type FieldError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fmt.Sprintf("%s: %s", fe.Field, fe.Message)
	}
	return "config validation failed: " + strings.Join(msgs, "; ")
}

// ErrInvalidConfig is the sentinel for configuration errors.
var ErrInvalidConfig = fmt.Errorf("invalid config")
