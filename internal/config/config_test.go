package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAgentEntry_UnmarshalYAML_BoolTrue(t *testing.T) {
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte("claude-code: true"), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	assert.Nil(t, m["claude-code"].Options.ProjectDir)
	assert.Nil(t, m["claude-code"].Options.Extra)
}

func TestAgentEntry_UnmarshalYAML_BoolFalse(t *testing.T) {
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte("pi: false"), &m)
	require.NoError(t, err)
	require.NotNil(t, m["pi"])
	assert.False(t, m["pi"].Enabled)
}

func TestAgentEntry_UnmarshalYAML_ObjectWithKnownField(t *testing.T) {
	input := "claude-code:\n  project_dir: .custom/skills/\n"
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	require.NotNil(t, m["claude-code"].Options.ProjectDir)
	assert.Equal(t, ".custom/skills/", *m["claude-code"].Options.ProjectDir)
}

func TestAgentEntry_UnmarshalYAML_ObjectWithExtraKey(t *testing.T) {
	input := "claude-code:\n  question_tool: AskUserQuestion\n"
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	assert.Nil(t, m["claude-code"].Options.ProjectDir)
	require.NotNil(t, m["claude-code"].Options.Extra)
	assert.Equal(t, "AskUserQuestion", m["claude-code"].Options.Extra["question_tool"])
}

func TestAgentEntry_UnmarshalYAML_ObjectWithKnownAndExtraKeys(t *testing.T) {
	input := "claude-code:\n  project_dir: .custom/\n  question_tool: AskUserQuestion\n"
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	e := m["claude-code"]
	require.NotNil(t, e)
	assert.True(t, e.Enabled)
	require.NotNil(t, e.Options.ProjectDir)
	assert.Equal(t, ".custom/", *e.Options.ProjectDir)
	// project_dir should NOT appear in Extra — it's consumed by the known field
	assert.Nil(t, e.Options.Extra["project_dir"])
	assert.Equal(t, "AskUserQuestion", e.Options.Extra["question_tool"])
}

func TestAgentEntry_UnmarshalYAML_InvalidKind(t *testing.T) {
	input := "claude-code:\n  - item1\n"
	var m map[string]*AgentEntry
	err := yaml.Unmarshal([]byte(input), &m)
	require.Error(t, err)
}
