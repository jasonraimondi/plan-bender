package template

import (
	"os"
	"path/filepath"
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
		"custom_fields":       []map[string]any{},
		"track_descriptions":  []map[string]string{},
		"agent":               "claude-code",
		"review_with_user":    false,
		"report_bugs":         false,
		"interview_with_docs": false,
		"commands": map[string]string{
			"context":         "plan-bender-agent context",
			"validate":        "plan-bender-agent validate",
			"write_prd":       "plan-bender-agent write-prd",
			"write_issue":     "plan-bender-agent write-issue",
			"sync_push":       "plan-bender-agent sync linear push",
			"sync_pull":       "plan-bender-agent sync linear pull",
			"archive":         "plan-bender-agent archive",
			"next":            "plan-bender-agent next",
			"status":          "plan-bender-agent status",
			"dispatch":        "plan-bender-agent dispatch",
			"complete":        "plan-bender-agent complete",
			"retry":           "plan-bender-agent retry",
			"worktree_create": "plan-bender-agent worktree create",
		},
	}
}

func TestAllTemplatesLoad(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	expected := []string{
		"bender-orchestrator",
		"bender-write-plan",
		"bender-write-prd",
		"bender-write-issue",
		"bender-prd-to-issues",
		"bender-review-prd",
		"bender-implement-prd",
		"bender-implement-hitl",
		"bender-implement-issue",
		"bender-interview-me",
		"bender-sync-linear",
		"bender-retrospective",
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
	for name, skill := range tmpls {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err, "template %s failed to render", name)
			assert.NotEmpty(t, out)
		})
	}
}

func TestImplementHitlTemplate_AgentConditional(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	tmplContent := tmpls["bender-implement-hitl"].Main()

	t.Run("claude-code uses AskUserQuestionTool", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "claude-code"
		out, err := Render("implement-hitl", tmplContent, ctx)
		require.NoError(t, err)
		assert.Contains(t, out, "AskUserQuestionTool")
	})

	t.Run("openclaw uses conversational phrasing", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "openclaw"
		out, err := Render("implement-hitl", tmplContent, ctx)
		require.NoError(t, err)
		assert.NotContains(t, out, "AskUserQuestionTool")
		assert.Contains(t, out, "Ask the user directly in conversation")
	})
}

func TestReviewPrdTemplate_AgentConditional(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	tmplContent := tmpls["bender-review-prd"].Main()

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
	out, err := Render("orchestrator", tmpls["bender-orchestrator"].Main(), ctx)
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
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)
	assert.NotContains(t, out, "Linear sync")
}

func TestImplementPrdTemplate_HasLinearSyncWhenEnabled(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = true
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "Linear sync")
}

func TestWritePrdTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-prd", tmpls["bender-write-prd"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-prd")
}

func TestPrdToIssuesTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("prd-to-issues", tmpls["bender-prd-to-issues"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-issue")
}

func TestWriteIssueTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-issue", tmpls["bender-write-issue"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-issue")
}

func TestWritePrdTemplate_ConditionalReviewStep(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	tmplContent := tmpls["bender-write-prd"].Main()

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
	tmplContent := tmpls["bender-write-issue"].Main()

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

func TestAllTemplates_ConditionalBugReportSection(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	const marker = "## Bug reports"

	for name, skill := range tmpls {
		t.Run(name+"/off", func(t *testing.T) {
			ctx := fixtureContext()
			ctx["report_bugs"] = false
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err)
			assert.NotContains(t, out, marker)
		})
		t.Run(name+"/on", func(t *testing.T) {
			ctx := fixtureContext()
			ctx["report_bugs"] = true
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err)
			assert.Contains(t, out, marker)
			if name != "bender-interview-me" {
				assert.Contains(t, out, "https://github.com/jasonraimondi/plan-bender/issues")
			}
		})
	}
}

func TestInterviewMeTemplate_InterviewWithDocsBlock(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	content := tmpls["bender-interview-me"].Main()

	t.Run("off renders identical to absent and omits the domain block", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["interview_with_docs"] = false
		off, err := Render("interview", content, ctx)
		require.NoError(t, err)

		absent := fixtureContext()
		delete(absent, "interview_with_docs")
		none, err := Render("interview", content, absent)
		require.NoError(t, err)

		assert.Equal(t, none, off, "false must render byte-identical to an absent flag")
		assert.NotContains(t, off, "Domain awareness")
	})

	t.Run("on appends the grill block", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["interview_with_docs"] = true
		out, err := Render("interview", content, ctx)
		require.NoError(t, err)

		assert.Contains(t, out, "one at a time")
		assert.Contains(t, out, "## Domain awareness")
		assert.Contains(t, out, "Update CONTEXT.md inline")
		assert.Contains(t, out, "Offer ADRs sparingly")
		assert.Contains(t, out, "docs/adr/")
	})
}

func TestWritePrdTemplate_InterviewWithDocsBlock(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)
	content := tmpls["bender-write-prd"].Main()

	t.Run("off renders identical to absent without the grill block", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["interview_with_docs"] = false
		off, err := Render("write-prd", content, ctx)
		require.NoError(t, err)

		absent := fixtureContext()
		delete(absent, "interview_with_docs")
		none, err := Render("write-prd", content, absent)
		require.NoError(t, err)

		assert.Equal(t, none, off, "false must render byte-identical to an absent flag")
		assert.NotContains(t, off, "Domain awareness")
	})

	t.Run("on appends the grill block before Process", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["interview_with_docs"] = true
		out, err := Render("write-prd", content, ctx)
		require.NoError(t, err)

		assert.Contains(t, out, "## Domain awareness")
		assert.Contains(t, out, "Update CONTEXT.md inline")
		assert.Contains(t, out, "Offer ADRs sparingly")
		assert.Less(t, strings.Index(out, "## Domain awareness"), strings.Index(out, "## Process"),
			"grill block must precede the Process section")
	})
}

func TestSyncCommands_RenderWithLinearTool(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = true

	cases := map[string]string{
		"bender-prd-to-issues": "plan-bender-agent sync linear push",
		"bender-write-issue":   "plan-bender-agent sync linear push",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, tmpls[name].Main(), ctx)
			require.NoError(t, err)
			assert.Contains(t, out, want)
			assert.NotContains(t, out, "{{.commands.sync}}")
		})
	}

	out, err := Render("bender-orchestrator", tmpls["bender-orchestrator"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent sync linear push")
	assert.Contains(t, out, "plan-bender-agent sync linear pull")
}

func TestImplementPrdTemplate_NoLongerHandRollsExecutionQueue(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)
	// Resolver/queue logic moved into Go (dispatch). Prose must not re-derive it.
	assert.NotContains(t, out, "Build the execution queue")
	assert.NotContains(t, out, "Routing rules:")
}

func TestOrchestratorTemplate_SuggestsNextResolver(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("orchestrator", tmpls["bender-orchestrator"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent next")
}

func TestWorkflowStatesJoin(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, strings.Join(ctx["workflow_states"].([]string), " → "))
}

func TestImplementHitlTemplate_UsesResolverAndIssueWorkflow(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-hitl", tmpls["bender-implement-hitl"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent next")
	assert.Contains(t, out, "plan-bender-agent status")
	assert.Contains(t, out, "plan-bender-agent validate")
	assert.Contains(t, out, "plan-bender-agent worktree create")
	assert.Contains(t, out, "plan-bender-agent complete")
	assert.Contains(t, out, "/bender-implement-prd")
}

func TestImplementPrdTemplate_SuggestsHitlSkill(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "/bender-implement-hitl")
}

func TestImplementPrdTemplate_DelegatesToDispatch(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent dispatch")

	assert.NotContains(t, out, "git worktree add")
	assert.NotContains(t, out, "git worktree remove")
	assert.NotContains(t, out, "git merge --no-ff")
	assert.NotContains(t, out, "ultrathink")

	assert.Contains(t, out, "Open the combined PR")
}

func TestImplementIssueTemplate_DiscoversViaNextAndSkipsPrUnderPrd(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent next")
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "do not push")
}

func TestImplementIssueTemplate_CallsCompleteSentinel(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "plan-bender-agent complete")
}

func TestLoadTemplates_IgnoresUnknownOverrideDir(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".plan-bender", "templates", "brand-new-skill")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTES.md"), []byte("x"), 0o644))

	skills, err := LoadTemplates(dir)
	require.NoError(t, err, "stray override dir must not block loading other skills")
	assert.NotContains(t, skills, "brand-new-skill", "unknown override dir must not be promoted to a phantom skill")
	assert.Contains(t, skills, "bender-write-prd", "bundled skills must still load")
}

func TestLoadTemplates_ErrorsOnOutputCollision(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTE.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTE.md.tmpl"), []byte("b"), 0o644))

	_, err := LoadTemplates(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both produce")
}

// A verbatim SKILL.md would shadow the rendered body output of SKILL.md.tmpl,
// silently swapping the skill's main content. validate() must catch this.
func TestLoadTemplates_ErrorsWhenVerbatimSkillShadowsBody(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "SKILL.md"), []byte("shadow"), 0o644))

	_, err := LoadTemplates(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both produce")
	assert.Contains(t, err.Error(), "SKILL.md")
}
