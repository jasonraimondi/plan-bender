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

func TestMerge_AgentsReplacement(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{
		Agents: []string{"claude-code", "openclaw"},
	})
	assert.Equal(t, []string{"claude-code", "openclaw"}, result.Agents)
}

func TestMerge_AgentsNilPreservesDefault(t *testing.T) {
	base := Defaults()
	result := merge(base, PartialConfig{})
	assert.Equal(t, []string{"claude-code"}, result.Agents)
}

func TestMerge_DefaultsImmutable(t *testing.T) {
	before := Defaults()
	base := Defaults()
	_ = merge(base, PartialConfig{MaxPoints: ptr(99)})
	assert.Equal(t, before, Defaults())
}
