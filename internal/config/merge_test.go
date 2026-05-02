package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

func TestMerge_EmptyOverride(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.Equal(t, Defaults(), result)
}

func TestMerge_ScalarOverwrite(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		MaxPoints: ptr(5),
		PlansDir:  ptr("./custom/"),
	})
	assert.Equal(t, 5, result.MaxPoints)
	assert.Equal(t, "./custom/", result.PlansDir)
}

func TestMerge_ArrayReplacement(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Tracks: []string{"alpha", "beta"},
	})
	assert.Equal(t, []string{"alpha", "beta"}, result.Tracks)
	// default workflow_states untouched
	assert.Equal(t, Defaults().WorkflowStates, result.WorkflowStates)
}

func TestMerge_NestedObjectMerge(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Linear: &LinearConfig{
			APIKey: "sk-test",
		},
	})
	assert.Equal(t, "sk-test", result.Linear.APIKey)
	assert.Equal(t, "", result.Linear.Team)
}

func TestMerge_LinearEnabled(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Linear: &LinearConfig{Enabled: true},
	})
	assert.True(t, result.Linear.Enabled)
}

func TestMerge_LinearEnabledPreserved(t *testing.T) {
	base := Defaults()
	base.Linear.Enabled = true
	result := merge(base, PartialConfig{
		Linear: &LinearConfig{APIKey: "sk-test"},
	})
	assert.True(t, result.Linear.Enabled)
	assert.Equal(t, "sk-test", result.Linear.APIKey)
}

func TestMerge_PipelineSkipReplaces(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Pipeline: &PipelineConfig{Skip: []string{"lint"}},
	})
	assert.Equal(t, []string{"lint"}, result.Pipeline.Skip)
}

func TestMerge_IssueSchemaCustomFields(t *testing.T) {
	base := Defaults()
	fields := []CustomFieldDef{{Name: "team", Type: "string", Required: true}}
	result := merge(base, PartialConfig{
		IssueSchema: &IssueSchemaConfig{CustomFields: fields},
	})
	require.Len(t, result.IssueSchema.CustomFields, 1)
	assert.Equal(t, "team", result.IssueSchema.CustomFields[0].Name)
}

func TestMerge_LinearStatusMap(t *testing.T) {
	base := Defaults()
	base.Linear.StatusMap = map[string]string{"todo": "To Do"}
	result := merge(base, PartialConfig{
		Linear: &LinearConfig{
			StatusMap: map[string]string{"done": "Done"},
		},
	})
	// map merges: both keys present
	assert.Equal(t, "To Do", result.Linear.StatusMap["todo"])
	assert.Equal(t, "Done", result.Linear.StatusMap["done"])
}

func TestMerge_ThreeLayerOrdering(t *testing.T) {
	base := Defaults()
	layer1 := PartialConfig{MaxPoints: ptr(5)}
	layer2 := PartialConfig{MaxPoints: ptr(8)}
	result := merge(merge(base, layer1), layer2)
	assert.Equal(t, 8, result.MaxPoints)
}

func TestMerge_UpdateCheckDefaultTrue(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.True(t, result.UpdateCheck)
}

func TestMerge_UpdateCheckOverrideToFalse(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{UpdateCheck: ptr(false)})
	assert.False(t, result.UpdateCheck)
}

func TestMerge_ManageGitignoreDefaultFalse(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.False(t, result.ManageGitignore)
}

func TestMerge_ManageGitignoreOverrideToTrue(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{ManageGitignore: ptr(true)})
	assert.True(t, result.ManageGitignore)
}

func TestMerge_AgentsPerKeyMerge(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Agents: map[string]*AgentEntry{"pi": {Enabled: true}},
	})
	// Both claude-code (from base) and pi (from layer) are present
	assert.NotNil(t, result.rawAgents["claude-code"])
	assert.NotNil(t, result.rawAgents["pi"])
}

func TestMerge_AgentsNilLayerPreservesBase(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.NotNil(t, result.rawAgents["claude-code"])
	assert.Len(t, result.rawAgents, 1)
}

func TestMerge_AgentsPerKeyOverride(t *testing.T) {
	base := Defaults()
	customDir := ".custom/skills/"
	result := merge(base, PartialConfig{
		Agents: map[string]*AgentEntry{
			"claude-code": {Enabled: true, Options: AgentOptions{ProjectDir: &customDir}},
		},
	})
	e := result.rawAgents["claude-code"]
	require.NotNil(t, e)
	require.NotNil(t, e.Options.ProjectDir)
	assert.Equal(t, customDir, *e.Options.ProjectDir)
}

func TestMerge_DefaultsImmutable(t *testing.T) {
	before := Defaults()
	base := Defaults()
	_ = merge(base, PartialConfig{MaxPoints: ptr(99)})
	assert.Equal(t, before, Defaults())
}

func TestMerge_HooksAcrossLayers(t *testing.T) {
	base := Defaults()
	layer1 := PartialConfig{Hooks: &HooksConfig{BeforeIssue: "go mod tidy"}}
	layer2 := PartialConfig{Hooks: &HooksConfig{AfterBatch: "make ci"}}
	result := merge(merge(base, layer1), layer2)
	assert.Equal(t, "go mod tidy", result.Hooks.BeforeIssue)
	assert.Equal(t, "make ci", result.Hooks.AfterBatch)
	assert.Equal(t, "", result.Hooks.AfterIssue)
}

func TestMerge_HooksOverrideKeepsOtherFields(t *testing.T) {
	base := Defaults()
	base.Hooks.BeforeIssue = "echo before"
	base.Hooks.AfterIssue = "echo after"
	result := merge(base, PartialConfig{Hooks: &HooksConfig{BeforeIssue: "echo new"}})
	assert.Equal(t, "echo new", result.Hooks.BeforeIssue)
	assert.Equal(t, "echo after", result.Hooks.AfterIssue)
}

func TestMerge_BranchStrategyDefault(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.Equal(t, "integration", result.Pipeline.BranchStrategy)
}

func TestMerge_BranchStrategyOverride(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{Pipeline: &PipelineConfig{BranchStrategy: "direct"}})
	assert.Equal(t, "direct", result.Pipeline.BranchStrategy)
	// existing skip preserved
	assert.Equal(t, []string{}, result.Pipeline.Skip)
}

func TestMerge_PipelineSkipPreservesBranchStrategy(t *testing.T) {
	base := Defaults()
	base.Pipeline.BranchStrategy = "direct"
	result := merge(base, PartialConfig{Pipeline: &PipelineConfig{Skip: []string{"lint"}}})
	assert.Equal(t, "direct", result.Pipeline.BranchStrategy)
	assert.Equal(t, []string{"lint"}, result.Pipeline.Skip)
}
