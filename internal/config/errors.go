package config

import (
	"fmt"
	"strings"
)

type ConfigError struct {
	Errors []FieldError
}

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

func (e *ConfigError) FormatHuman() string {
	var b strings.Builder
	b.WriteString("Config errors:\n")
	for _, fe := range e.Errors {
		fmt.Fprintf(&b, "  \u2022 %s: %s\n", fe.Field, fe.Message)
	}
	return b.String()
}

var ErrInvalidConfig = fmt.Errorf("invalid config")
