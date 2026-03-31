package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigError_FormatHuman_SingleError(t *testing.T) {
	err := &ConfigError{Errors: []FieldError{{Field: "tracks", Message: "must not be empty"}}}
	output := err.FormatHuman()
	assert.Contains(t, output, "tracks: must not be empty")
}

func TestConfigError_FormatHuman_MultipleErrors(t *testing.T) {
	err := &ConfigError{Errors: []FieldError{
		{Field: "tracks", Message: "must not be empty"},
		{Field: "max_points", Message: "must be at least 1"},
	}}
	output := err.FormatHuman()
	assert.Contains(t, output, "tracks")
	assert.Contains(t, output, "max_points")
}

func TestConfigError_Error_Unchanged(t *testing.T) {
	err := &ConfigError{Errors: []FieldError{{Field: "tracks", Message: "must not be empty"}}}
	assert.Contains(t, err.Error(), "config validation failed: tracks: must not be empty")
}
