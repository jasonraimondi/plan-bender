package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_DefaultsPasses(t *testing.T) {
	cfg := Defaults()
	err := validate(&cfg)
	assert.NoError(t, err)
}

func TestValidate_InvalidBackend(t *testing.T) {
	cfg := Defaults()
	cfg.Backend = "nope"
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "backend")
}

func TestValidate_EmptyTracks(t *testing.T) {
	cfg := Defaults()
	cfg.Tracks = []string{}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "tracks")
}

func TestValidate_ZeroMaxPoints(t *testing.T) {
	cfg := Defaults()
	cfg.MaxPoints = 0
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "max_points")
}

func TestValidate_LinearRequiresAPIKeyAndTeam(t *testing.T) {
	cfg := Defaults()
	cfg.Backend = BackendLinear
	err := validate(&cfg)
	require.Error(t, err)

	ce := err.(*ConfigError)
	assert.Len(t, ce.Errors, 2)
	assertFieldError(t, err, "linear.api_key")
	assertFieldError(t, err, "linear.team")
}

func TestValidate_LinearWithCredsPasses(t *testing.T) {
	cfg := Defaults()
	cfg.Backend = BackendLinear
	cfg.Linear.APIKey = "lin_api_key"
	cfg.Linear.Team = "my-team"
	err := validate(&cfg)
	assert.NoError(t, err)
}

func TestValidate_CustomFieldEmptyName(t *testing.T) {
	cfg := Defaults()
	cfg.IssueSchema.CustomFields = []CustomFieldDef{{Name: "", Type: "string"}}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "issue_schema.custom_fields[0].name")
}

func TestValidate_EnumRequiresValues(t *testing.T) {
	cfg := Defaults()
	cfg.IssueSchema.CustomFields = []CustomFieldDef{{Name: "priority", Type: "enum"}}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "issue_schema.custom_fields[0].enum_values")
}

func TestValidate_EmptyAgents(t *testing.T) {
	cfg := Defaults()
	cfg.Agents = []string{}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "agents")
}

func TestValidate_UnknownAgent(t *testing.T) {
	cfg := Defaults()
	cfg.Agents = []string{"claude-code", "unknown-agent"}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "agents[1]")
}

func TestValidate_DuplicateAgentsDeduplicated(t *testing.T) {
	cfg := Defaults()
	cfg.Agents = []string{"claude-code", "claude-code"}
	err := validate(&cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code"}, cfg.Agents)
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := Defaults()
	cfg.Tracks = []string{}
	cfg.MaxPoints = 0
	err := validate(&cfg)
	require.Error(t, err)

	ce := err.(*ConfigError)
	assert.GreaterOrEqual(t, len(ce.Errors), 2)
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	ce, ok := err.(*ConfigError)
	require.True(t, ok, "expected *ConfigError, got %T", err)
	for _, fe := range ce.Errors {
		if fe.Field == field {
			return
		}
	}
	t.Errorf("expected field error for %q, got: %v", field, ce.Errors)
}
