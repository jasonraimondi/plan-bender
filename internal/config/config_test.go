package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentEntry_UnmarshalJSON_BoolTrue(t *testing.T) {
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(`{"claude-code": true}`), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	assert.Nil(t, m["claude-code"].Options.ProjectDir)
	assert.Nil(t, m["claude-code"].Options.Extra)
}

func TestAgentEntry_UnmarshalJSON_BoolFalse(t *testing.T) {
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(`{"pi": false}`), &m)
	require.NoError(t, err)
	require.NotNil(t, m["pi"])
	assert.False(t, m["pi"].Enabled)
}

func TestAgentEntry_UnmarshalJSON_ObjectWithKnownField(t *testing.T) {
	input := `{"claude-code": {"project_dir": ".custom/skills/"}}`
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	require.NotNil(t, m["claude-code"].Options.ProjectDir)
	assert.Equal(t, ".custom/skills/", *m["claude-code"].Options.ProjectDir)
}

func TestAgentEntry_UnmarshalJSON_ObjectWithExtraKey(t *testing.T) {
	input := `{"claude-code": {"question_tool": "AskUserQuestion"}}`
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	require.NotNil(t, m["claude-code"])
	assert.True(t, m["claude-code"].Enabled)
	assert.Nil(t, m["claude-code"].Options.ProjectDir)
	require.NotNil(t, m["claude-code"].Options.Extra)
	assert.Equal(t, "AskUserQuestion", m["claude-code"].Options.Extra["question_tool"])
}

func TestAgentEntry_UnmarshalJSON_ObjectWithKnownAndExtraKeys(t *testing.T) {
	input := `{"claude-code": {"project_dir": ".custom/", "question_tool": "AskUserQuestion"}}`
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(input), &m)
	require.NoError(t, err)
	e := m["claude-code"]
	require.NotNil(t, e)
	assert.True(t, e.Enabled)
	require.NotNil(t, e.Options.ProjectDir)
	assert.Equal(t, ".custom/", *e.Options.ProjectDir)
	assert.Nil(t, e.Options.Extra["project_dir"])
	assert.Equal(t, "AskUserQuestion", e.Options.Extra["question_tool"])
}

func TestAgentEntry_UnmarshalJSON_InvalidKind(t *testing.T) {
	input := `{"claude-code": ["item1"]}`
	var m map[string]*AgentEntry
	err := json.Unmarshal([]byte(input), &m)
	require.Error(t, err)
}

func TestDefaults_InterviewWithDocsFalse(t *testing.T) {
	assert.False(t, Defaults().InterviewWithDocs)
}

func TestPartialConfig_InterviewWithDocsRoundTrip(t *testing.T) {
	pc := PartialConfig{InterviewWithDocs: ptr(true)}
	data, err := json.Marshal(pc)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"interview_with_docs":true`)

	var got PartialConfig
	require.NoError(t, json.Unmarshal(data, &got))
	require.NotNil(t, got.InterviewWithDocs)
	assert.True(t, *got.InterviewWithDocs)
}

func TestPartialConfig_InterviewWithDocsOmittedWhenNil(t *testing.T) {
	data, err := json.Marshal(PartialConfig{})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "interview_with_docs")
}

func TestDefaults_NoImplementFalse(t *testing.T) {
	assert.False(t, Defaults().NoImplement)
}

func TestPartialConfig_NoImplementRoundTrip(t *testing.T) {
	pc := PartialConfig{NoImplement: ptr(true)}
	data, err := json.Marshal(pc)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"no_implement":true`)

	var got PartialConfig
	require.NoError(t, json.Unmarshal(data, &got))
	require.NotNil(t, got.NoImplement)
	assert.True(t, *got.NoImplement)
}

func TestPartialConfig_NoImplementOmittedWhenNil(t *testing.T) {
	data, err := json.Marshal(PartialConfig{})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "no_implement")
}
