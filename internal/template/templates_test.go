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
		"agent":            "claude-code",
		"review_with_user": false,
		"commands": map[string]string{
			"context":     "plan-bender-agent context",
			"validate":    "plan-bender-agent validate",
			"write_prd":   "plan-bender-agent write-prd",
			"write_issue": "plan-bender-agent write-issue",
			"sync_push":   "plan-bender-agent sync linear push",
			"sync_pull":   "plan-bender-agent sync linear pull",
			"archive":     "plan-bender-agent archive",
			"next":        "plan-bender-agent next",
			"dispatch":    "plan-bender-agent dispatch",
			"complete":    "plan-bender-agent complete",
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
		"bender-sync-linear.skill.tmpl",
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
	assert.Contains(t, out, "plan-bender-agent write-prd")
}

func TestPrdToIssuesTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("prd-to-issues", tmpls["bender-prd-to-issues.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-issue")
}

func TestWriteIssueTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-issue", tmpls["bender-write-issue.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-issue")
}

func TestWritePrdTemplate_ConditionalReviewStep(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	tmplContent := tmpls["bender-write-prd.skill.tmpl"]

	t.Run("with review", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["review_with_user"] = true
		out, err := Render("write-prd", tmplContent, ctx)
		require.NoError(t, err)
		assert.Contains(t, out, "Review with the user")
	})

	t.Run("without review", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["review_with_user"] = false
		out, err := Render("write-prd", tmplContent, ctx)
		require.NoError(t, err)
		assert.NotContains(t, out, "Review with the user")
	})
}

func TestWriteIssueTemplate_ConditionalReviewStep(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	tmplContent := tmpls["bender-write-issue.skill.tmpl"]

	t.Run("with review", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["review_with_user"] = true
		out, err := Render("write-issue", tmplContent, ctx)
		require.NoError(t, err)
		assert.Contains(t, out, "Review with the user")
	})

	t.Run("without review", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["review_with_user"] = false
		out, err := Render("write-issue", tmplContent, ctx)
		require.NoError(t, err)
		assert.NotContains(t, out, "Review with the user")
	})
}

func TestSyncCommands_RenderWithLinearTool(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = true

	cases := map[string]string{
		"bender-prd-to-issues.skill.tmpl": "plan-bender-agent sync linear push",
		"bender-write-issue.skill.tmpl":   "plan-bender-agent sync linear push",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, tmpls[name], ctx)
			require.NoError(t, err)
			assert.Contains(t, out, want)
			assert.NotContains(t, out, "{{.commands.sync}}")
		})
	}

	// Orchestrator surfaces both push and pull
	out, err := Render("bender-orchestrator.skill.tmpl", tmpls["bender-orchestrator.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent sync linear push")
	assert.Contains(t, out, "plan-bender-agent sync linear pull")
}

func TestImplementPrdTemplate_NoLongerHandRollsExecutionQueue(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd.skill.tmpl"], ctx)
	require.NoError(t, err)
	// Resolver/queue logic moved into Go (dispatch). Prose must not re-derive it.
	assert.NotContains(t, out, "Build the execution queue")
	assert.NotContains(t, out, "Routing rules:")
}

func TestOrchestratorTemplate_SuggestsNextResolver(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("orchestrator", tmpls["bender-orchestrator.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent next")
}

func TestWorkflowStatesJoin(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue.skill.tmpl"], ctx)
	require.NoError(t, err)
	assert.Contains(t, out, strings.Join(ctx["workflow_states"].([]string), " → "))
}

func TestImplementPrdTemplate_DelegatesToDispatch(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd.skill.tmpl"], ctx)
	require.NoError(t, err)

	// Skill body is now one dispatch call instead of inline worktree prose.
	assert.Contains(t, out, "plan-bender-agent dispatch")

	// All parallel-worktree and merge-back prose lives in Go now.
	assert.NotContains(t, out, "git worktree add")
	assert.NotContains(t, out, "git worktree remove")
	assert.NotContains(t, out, "git merge --no-ff")
	assert.NotContains(t, out, "ultrathink")

	// Combined PR section (§6) is preserved for the human to land the work.
	assert.Contains(t, out, "Open the combined PR")
}

func TestImplementIssueTemplate_DiscoversViaNextAndSkipsPrUnderPrd(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue.skill.tmpl"], ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent next")
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "do not push")
}

func TestImplementIssueTemplate_CallsCompleteSentinel(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue.skill.tmpl"], ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent complete")
}
