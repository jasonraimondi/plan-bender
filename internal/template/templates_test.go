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
			{"name": "Write Plan", "description": "Create a PRD and decompose it into issues in one pass", "skill": "bender-write-plan"},
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
			"merge":           "plan-bender-agent merge",
			"retry":           "plan-bender-agent retry",
			"park":            "plan-bender-agent park",
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
		"bender-write-issue",
		"bender-review-prd",
		"bender-implement-prd",
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
	assert.Contains(t, out, "Write Plan")
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

func TestWriteIssueTemplate_UsesCLIWriteThrough(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("write-issue", tmpls["bender-write-issue"].Main(), ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "plan-bender-agent write-issue")
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

func TestSyncCommands_RenderWithLinearTool(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	ctx["has_backend_sync"] = true

	cases := map[string]string{
		"bender-write-plan":  "plan-bender-agent sync linear push",
		"bender-write-issue": "plan-bender-agent sync linear push",
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

func TestImplementPrdTemplate_BatchesNeedsInputInterview(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	// The needs-input decisions are resolved with the user and resumed via retry,
	// not handed off to a separate HITL skill.
	assert.NotContains(t, out, "/bender-implement-hitl")
	assert.Contains(t, out, "needs-input")
	assert.Contains(t, out, "plan-bender-agent retry")
}

func TestImplementPrdTemplate_DrivesWorkflowLoop(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-prd", tmpls["bender-implement-prd"].Main(), ctx)
	require.NoError(t, err)

	// The dispatch subcommand was removed; the dispatcher now drives the
	// harness Workflow tool through a scout → workers → merger loop.
	assert.NotContains(t, out, "plan-bender-agent dispatch")
	assert.Contains(t, out, "Workflow")
	assert.Contains(t, out, "plan-bender-agent merge")
	assert.Contains(t, out, "plan-bender-agent context")
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

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 7.")
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

	mergeBlock := sliceBetween(out, "If you chose `merge`", "If you chose `branch`", "If you chose `pr`", "### 7.")
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

	prBlock := sliceBetween(out, "If you chose `pr`", "### 7.")
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

	branchBlock := sliceBetween(out, "If you chose `branch`", "If you chose `pr`", "If you chose `merge`", "### 7.")
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

	report := sliceBetween(out, "### 7. Status report", "Update `prd.json`")
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

func TestImplementIssueTemplate_DiscoversViaNextAndDoesNotIntegrate(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	// Discovers a ready issue from the slug.
	assert.Contains(t, out, "plan-bender-agent next")
	// The worker never integrates — no push or merge commands; that's the dispatcher's merger.
	assert.NotContains(t, out, "git push")
	assert.NotContains(t, out, "git merge")
}

func TestImplementIssueTemplate_WarmWorkerContract(t *testing.T) {
	tmpls, err := LoadTemplates(t.TempDir())
	require.NoError(t, err)

	ctx := fixtureContext()
	out, err := Render("implement-issue", tmpls["bender-implement-issue"].Main(), ctx)
	require.NoError(t, err)

	// Claims the issue via worktree create and uses the three structured outcomes.
	assert.Contains(t, out, "plan-bender-agent worktree create")
	assert.Contains(t, out, "completed")
	assert.Contains(t, out, "blocked")
	assert.Contains(t, out, "needs-decision")
	// Escalation is via park (needs-input), not a guess.
	assert.Contains(t, out, "plan-bender-agent park")
	assert.Contains(t, out, "needs-input")

	// No residue of the removed cold-subprocess / mode framing.
	assert.NotContains(t, out, "INTEGRATION mode")
	assert.NotContains(t, out, "STANDALONE mode")
	assert.NotContains(t, out, "--print")
	assert.NotContains(t, out, "plan-bender-agent dispatch")
}

func TestImplementIssueTemplate_CallsComplete(t *testing.T) {
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
	assert.Contains(t, skills, "bender-write-plan", "bundled skills must still load")
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
