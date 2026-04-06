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

func TestValidate_LinearEnabledRequiresAPIKeyAndTeam(t *testing.T) {
	cfg := Defaults()
	cfg.Linear.Enabled = true
	err := validate(&cfg)
	require.Error(t, err)

	ce := err.(*ConfigError)
	assert.Len(t, ce.Errors, 2)
	assertFieldError(t, err, "linear.api_key")
	assertFieldError(t, err, "linear.team")
}

func TestValidate_LinearEnabledWithCredsPasses(t *testing.T) {
	cfg := Defaults()
	cfg.Linear.Enabled = true
	cfg.Linear.APIKey = "lin_api_key"
	cfg.Linear.Team = "my-team"
	err := validate(&cfg)
	assert.NoError(t, err)
}

func TestValidate_LinearDisabledSkipsCredCheck(t *testing.T) {
	cfg := Defaults()
	cfg.Linear.Enabled = false
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

func TestValidate_AllDisabledAgentsError(t *testing.T) {
	cfg := Defaults()
	cfg.rawAgents = map[string]*AgentEntry{"claude-code": {Enabled: false}}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "agents")
}

func TestValidate_UnknownAgentInDisabledEntry(t *testing.T) {
	cfg := Defaults()
	cfg.rawAgents = map[string]*AgentEntry{
		"claude-code": {Enabled: true},
		"typo-agent":  {Enabled: false},
	}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "agents[typo-agent]")
}

func TestValidate_UnknownAgentEnabled(t *testing.T) {
	cfg := Defaults()
	cfg.rawAgents = map[string]*AgentEntry{
		"claude-code":   {Enabled: true},
		"unknown-agent": {Enabled: true},
	}
	err := validate(&cfg)
	require.Error(t, err)
	assertFieldError(t, err, "agents[unknown-agent]")
}

func TestValidate_AgentOptionsOverrideRegistryField(t *testing.T) {
	cfg := Defaults()
	customDir := ".custom/skills/"
	cfg.rawAgents = map[string]*AgentEntry{
		"claude-code": {Enabled: true, Options: AgentOptions{ProjectDir: &customDir}},
	}
	err := validate(&cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Agents, 1)
	assert.Equal(t, ".custom/skills/", cfg.Agents[0].ProjectDir)
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
