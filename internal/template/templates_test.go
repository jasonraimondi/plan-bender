package template

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureContext returns a template rendering context matching default config.
func fixtureContext() map[string]any {
	return map[string]any{
		"plans_dir":    "./.plan-bender/plans/",
		"max_points":   3,
		"step_pattern": "Target — behavior",
		"tracks":       []string{"intent", "experience", "data", "rules", "resilience"},
		"workflow_states": []string{
			"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled",
		},
		"has_backend_sync": false,
		"pipeline_phases": []map[string]string{
			{"name": "Interview", "description": "Stress-test your plan", "skill": "bender-interview-me"},
			{"name": "Write PRD", "description": "Create a PRD", "skill": "bender-write-prd"},
		},
		"custom_fields":     []map[string]any{},
		"track_descriptions": []map[string]string{},
		"agent": "claude-code",
		"commands": map[string]string{
			"context":     "plan-bender-agent context",
			"validate":    "plan-bender-agent validate",
			"write_prd":   "plan-bender-agent write-prd",
			"write_issue": "plan-bender-agent write-issue",
			"sync":        "plan-bender-agent sync",
			"archive":     "plan-bender-agent archive",
		},
	}
}

func TestAllTemplatesLoad(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	expected := []string{
		"bender-orchestrator.skill.tmpl",
		"bender-write-prd.skill.tmpl",
		"bender-write-issue.skill.tmpl",
		"bender-prd-to-issues.skill.tmpl",
		"bender-review-prd.skill.tmpl",
		"bender-implement-prd.skill.tmpl",
		"bender-implement-issue.skill.tmpl",
		"bender-interview-me.skill.tmpl",
	}
	for _, name := range expected {
		assert.Contains(t, tmpls, name, "missing template %s", name)
	}
	assert.Len(t, tmpls, len(expected))
}

func TestAllTemplatesRender(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	for name, content := range tmpls {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, content, ctx)
			require.NoError(t, err, "template %s failed to render", name)
			assert.NotEmpty(t, out)
		})
	}
}

func TestInterviewTemplate_AgentConditional(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	tmplContent := tmpls["bender-interview-me.skill.tmpl"]

	t.Run("claude-code uses AskUserQuestionTool", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "claude-code"
		out, err := Render("interview", tmplContent, ctx)
		require.NoError(t, err)
		assert.Contains(t, out, "AskUserQuestionTool")
	})

	t.Run("openclaw uses conversational phrasing", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "openclaw"
		out, err := Render("interview", tmplContent, ctx)
		require.NoError(t, err)
		assert.NotContains(t, out, "AskUserQuestionTool")
		assert.Contains(t, out, "Ask the user directly in conversation")
	})
}

func TestReviewPrdTemplate_AgentConditional(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	tmplContent := tmpls["bender-review-prd.skill.tmpl"]

	t.Run("claude-code uses AskUserQuestion", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "claude-code"
		out, err := Render("review-prd", tmplContent, ctx)
		require.NoError(t, err)
		assert.Contains(t, out, "AskUserQuestion")
	})

	t.Run("openclaw uses conversational phrasing", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "openclaw"
		out, err := Render("review-prd", tmplContent, ctx)
		require.NoError(t, err)
		assert.NotContains(t, out, "AskUserQuestion")
		assert.Contains(t, out, "Ask the user directly in conversation")
	})
}

func TestOrchestratorTemplate_ContainsPipelinePhases(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("orchestrator", tmpls["bender-orchestrator.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "Interview")
	assert.Contains(t, out, "Write PRD")
	assert.Contains(t, out, "./.plan-bender/plans/")
}

func TestImplementPrdTemplate_NoLinearSyncWhenDisabled(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = false
	out, err := Render("implement-prd", tmpls["bender-implement-prd.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.NotContains(t, out, "Linear sync")
}

func TestImplementPrdTemplate_HasLinearSyncWhenEnabled(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = true
	out, err := Render("implement-prd", tmpls["bender-implement-prd.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "Linear sync")
}

func TestWritePrdTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-prd", tmpls["bender-write-prd.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender write-prd")
}

func TestPrdToIssuesTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("prd-to-issues", tmpls["bender-prd-to-issues.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender write-issue")
}

func TestWriteIssueTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-issue", tmpls["bender-write-issue.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender write-issue")
}

func TestWorkflowStatesJoin(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, strings.Join(ctx["workflow_states"].([]string), " → "))
}
