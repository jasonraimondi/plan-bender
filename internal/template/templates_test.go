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
	assert.NotContains(t, out, "ultrathink")
}

// sliceBetween returns the substring of haystack between the first occurrence of
// start (inclusive) and the next occurrence of any string in stops (exclusive),
// or the rest of haystack if no stop is found. Returns "" if start is absent.
func sliceBetween(haystack, start string, stops ...string) string {
	i := strings.Index(haystack, start)
	if i < 0 {
		return ""
	}
	rest := haystack[i:]
	end := len(rest)
	for _, s := range stops {
		if j := strings.Index(rest[len(start):], s); j >= 0 && j+len(start) < end {
			end = j + len(start)
		}
	}
	return rest[:end]
}

func TestImplementPrdTemplate_PromptsForMode_ClaudeCode(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["agent"] = "claude-code"
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "AskUserQuestion")
	assert.Contains(t, out, "landing branch")
	assert.Contains(t, out, "git branch --show-current")
	assert.NotContains(t, out, "Ask the user directly in conversation")
}

func TestImplementPrdTemplate_PromptsForMode_Pi(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["agent"] = "pi"
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	assert.NotContains(t, out, "AskUserQuestion")
	assert.Contains(t, out, "Ask the user directly in conversation")
	assert.Contains(t, out, "landing branch")
	assert.Contains(t, out, "git branch --show-current")
}

func TestImplementPrdTemplate_MergeMode_EmitsGitMergeAndDelete(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 5.")
	require.NotEmpty(t, mergeBlock, "merge sub-section missing")
	assert.Contains(t, mergeBlock, "git merge --no-ff")
	assert.Contains(t, mergeBlock, "git branch -d")
}

func TestImplementPrdTemplate_MergeMode_IncludesConflictHint(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 5.")
	require.NotEmpty(t, mergeBlock, "merge sub-section missing")
	assert.Contains(t, mergeBlock, "merge conflicts",
		"merge block must include a one-line conflict-recovery hint")
	assert.Contains(t, mergeBlock, "integration branch is preserved",
		"hint must reassure the operator that the source branch survives a failed merge")
}

func TestImplementPrdTemplate_PrMode_PreservesPushAndGhPrCreate(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	prBlock := sliceBetween(out, "If you chose `pr`", "### 5.")
	require.NotEmpty(t, prBlock, "pr sub-section missing")
	assert.Contains(t, prBlock, "git push -u origin")
	assert.Contains(t, prBlock, "PR off `<user>/<slug>`")
	assert.Contains(t, prBlock, "default branch")
}

func TestImplementPrdTemplate_BranchMode_NoPushOrPR(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	branchBlock := sliceBetween(out, "If you chose `branch`", "If you chose `pr`", "If you chose `merge`", "### 5.")
	require.NotEmpty(t, branchBlock, "branch sub-section missing")
	assert.NotContains(t, branchBlock, "git push")
	assert.NotContains(t, branchBlock, "gh pr")
	assert.NotContains(t, branchBlock, "git merge")
}

func TestImplementPrdTemplate_StatusReport_ConditionalPRLine(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	report := sliceBetween(out, "### 5. Status report", "Update `prd.json`")
	require.NotEmpty(t, report, "status report section missing")

	prReport := sliceBetween(report, "If you chose `pr`", "If you chose `merge`", "If you chose `branch`")
	require.NotEmpty(t, prReport, "pr-mode status block missing")
	assert.Contains(t, prReport, "PR:")
	assert.NotContains(t, prReport, "Integration branch:")

	mergeBranchReport := sliceBetween(report, "If you chose `merge` or `branch`", "Update `prd.json`")
	require.NotEmpty(t, mergeBranchReport, "merge/branch status block missing")
	assert.Contains(t, mergeBranchReport, "Integration branch:")
	assert.NotContains(t, mergeBranchReport, "PR:")
}

func TestImplementPrdTemplate_HideMergeWhenLandingIsIntegration(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	preflight := sliceBetween(out, "### 2.", "### 3.")
	require.NotEmpty(t, preflight, "pre-flight section missing")
	assert.Contains(t, preflight, "working tree dirty",
		"dirty-tree hide reason must be documented")
	assert.Contains(t, preflight, "HEAD detached",
		"detached-HEAD hide reason must be documented")
	assert.Contains(t, preflight, "landing branch is the integration branch",
		"self-merge hide reason must be documented")
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

func TestImplementIssueTemplate_StandalonePromptsForMode(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	content := tmpls["bender-implement-issue"].Main()

	t.Run("claude-code uses AskUserQuestion", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "claude-code"
		out, err := Render("implement-issue", content, ctx)
		require.NoError(t, err)

		assert.Contains(t, out, "AskUserQuestion")
		assert.Contains(t, out, "landing branch")
		assert.Contains(t, out, "git branch --show-current")
		assert.NotContains(t, out, "Ask the user directly in conversation")
	})

	t.Run("pi uses conversational phrasing", func(t *testing.T) {
		ctx := fixtureContext()
		ctx["agent"] = "pi"
		out, err := Render("implement-issue", content, ctx)
		require.NoError(t, err)

		assert.NotContains(t, out, "AskUserQuestion")
		assert.Contains(t, out, "Ask the user directly in conversation")
		assert.Contains(t, out, "landing branch")
		assert.Contains(t, out, "git branch --show-current")
	})
}

// TestImplementIssueTemplate_WorktreeModeSkipsPrompt asserts the pre-flight
// prompt section is gated by a prose skip-marker that points to bender-implement-prd
// integration mode. The skill is rendered once at install time and read by both
// standalone and worktree-mode agents; the marker is what tells the worktree-mode
// agent to bypass the prompt section.
func TestImplementIssueTemplate_WorktreeModeSkipsPrompt(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	preflight := sliceBetween(out, "### 2.", "### 3.")
	require.NotEmpty(t, preflight, "pre-flight section missing")
	assert.Contains(t, preflight, "Skip this section when invoked under `bender-implement-prd` integration mode",
		"prompt section must carry the worktree-mode skip marker so dispatch sub-agents bypass it")
	// The distinguishing prompt mechanic (AskUserQuestion for claude-code) must
	// sit below the skip marker; an agent under worktree-mode that follows the
	// marker will not execute the prompt mechanic below it.
	skipIdx := strings.Index(preflight, "Skip this section when invoked under `bender-implement-prd`")
	promptIdx := strings.Index(preflight, "AskUserQuestion")
	require.GreaterOrEqual(t, skipIdx, 0, "skip marker missing")
	require.GreaterOrEqual(t, promptIdx, 0, "AskUserQuestion mechanic missing under claude-code agent")
	assert.Less(t, skipIdx, promptIdx,
		"skip marker must precede the prompt so worktree-mode readers bail before reaching it")
}

func TestImplementIssueTemplate_MergeMode_EmitsGitMergeAndWorktreeGc(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 9.")
	require.NotEmpty(t, mergeBlock, "merge sub-section missing")
	assert.Contains(t, mergeBlock, "git merge --no-ff")
	assert.Contains(t, mergeBlock, "plan-bender-agent worktree gc",
		"merge block must use worktree gc (which removes the worktree AND deletes the branch)")
	assert.NotContains(t, mergeBlock, "git branch -d",
		"raw `git branch -d` fails on a branch checked out in another worktree — must use `pba worktree gc` instead")
}

func TestImplementIssueTemplate_MergeMode_IncludesConflictHint(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 9.")
	require.NotEmpty(t, mergeBlock, "merge sub-section missing")
	assert.Contains(t, mergeBlock, "merge conflicts",
		"merge block must include a one-line conflict-recovery hint")
	assert.Contains(t, mergeBlock, "issue branch and worktree are preserved",
		"hint must reassure the operator that the per-issue branch and worktree survive a failed merge")
}

func TestImplementIssueTemplate_HideMergeWhenLandingIsIssueBranch(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	preflight := sliceBetween(out, "### 2.", "### 3.")
	require.NotEmpty(t, preflight, "pre-flight section missing")
	assert.Contains(t, preflight, "working tree dirty",
		"dirty-tree hide reason must be documented")
	assert.Contains(t, preflight, "HEAD detached",
		"detached-HEAD hide reason must be documented")
	assert.Contains(t, preflight, "landing branch is the issue branch",
		"self-merge hide reason must be documented (landing branch equals per-issue branch)")
}

func TestImplementIssueTemplate_PrMode_PreservesPushAndGhPrCreate(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	prBlock := sliceBetween(out, "If you chose `pr`", "### 9.")
	require.NotEmpty(t, prBlock, "pr sub-section missing")
	assert.Contains(t, prBlock, "Push the branch with `-u`",
		"pr block must preserve the original push instruction from the legacy §7")
	assert.Contains(t, prBlock, "Create a PR with the issue reference",
		"pr block must preserve the original PR creation instruction from the legacy §7")
	assert.Contains(t, prBlock, "test plan",
		"pr block must preserve the test-plan instruction from the legacy §7")
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
