package template

import "github.com/jasonraimondi/plan-bender/internal/config"

var defaultCommands = map[string]string{
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
}

var defaultTrackDescriptions = map[string]string{
	"intent":     "What the system should do — features, commands, API behavior",
	"experience": "How users interact with the system — UI, UX, accessibility",
	"data":       "Data models, schemas, storage, migrations, CRUD behavior",
	"rules":      "Business rules, validation, authorization, constraints",
	"resilience": "Error handling, retries, fallbacks, monitoring, recovery",
}

// PipelinePhase is a step in the planning pipeline.
type PipelinePhase struct {
	Name            string
	Description     string
	Skill           string
	RequiresBackend bool
	Implement       bool
}

// SkillRequiresBackend reports whether a skill template should only be
// generated when a backend (Linear) is enabled.
func SkillRequiresBackend(skill string) bool {
	for _, p := range defaultPipelinePhases {
		if p.Skill == skill {
			return p.RequiresBackend
		}
	}
	return false
}

// SkillIsImplement reports whether a skill template is an implementation skill,
// suppressed when no_implement is set.
func SkillIsImplement(skill string) bool {
	for _, p := range defaultPipelinePhases {
		if p.Skill == skill {
			return p.Implement
		}
	}
	return false
}

var defaultPipelinePhases = []PipelinePhase{
	{Name: "Interview", Description: "Stress-test your plan", Skill: "bender-interview-me"},
	{Name: "Write Plan", Description: "Create a PRD and decompose it into issues in one pass", Skill: "bender-write-plan"},
	{Name: "Write Issue", Description: "Create a single issue", Skill: "bender-write-issue"},
	{Name: "Review PRD", Description: "Review plan quality", Skill: "bender-review-prd"},
	{Name: "Implement PRD", Description: "Work through issues", Skill: "bender-implement-prd", Implement: true},
	{Name: "Implement HITL", Description: "Resolve human-gated issues", Skill: "bender-implement-hitl", Implement: true},
	{Name: "Implement Issue", Description: "Implement one issue", Skill: "bender-implement-issue", Implement: true},
	{Name: "Sync with Linear", Description: "Push local plan to Linear or pull Linear state", Skill: "bender-sync-linear", RequiresBackend: true},
}

// BuildContext produces the template rendering context from config for a specific agent.
// Extra keys from the agent are flat-merged first; built-in keys always win on collision.
func BuildContext(cfg config.Config, agent config.ResolvedAgent) map[string]any {
	ctx := make(map[string]any)

	for k, v := range agent.Extra {
		ctx[k] = v
	}

	tds := make([]map[string]string, len(cfg.Tracks))
	for i, t := range cfg.Tracks {
		desc, ok := defaultTrackDescriptions[t]
		if !ok {
			desc = t + " track"
		}
		tds[i] = map[string]string{"name": t, "description": desc}
	}

	skipSet := make(map[string]bool, len(cfg.Pipeline.Skip))
	for _, s := range cfg.Pipeline.Skip {
		skipSet[s] = true
	}
	var phases []map[string]string
	for _, p := range defaultPipelinePhases {
		if skipSet[p.Skill] {
			continue
		}
		if p.RequiresBackend && !cfg.Linear.Enabled {
			continue
		}
		if p.Implement && cfg.NoImplement {
			continue
		}
		phases = append(phases, map[string]string{
			"name":        p.Name,
			"description": p.Description,
			"skill":       p.Skill,
		})
	}

	cfs := make([]map[string]any, len(cfg.IssueSchema.CustomFields))
	for i, f := range cfg.IssueSchema.CustomFields {
		cfs[i] = map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"enum_values": f.EnumValues,
		}
	}

	ctx["plans_dir"] = cfg.PlansDir
	ctx["tracks"] = cfg.Tracks
	ctx["workflow_states"] = cfg.WorkflowStates
	ctx["step_pattern"] = "Target — behavior"
	ctx["max_points"] = cfg.MaxPoints
	ctx["has_backend_sync"] = cfg.Linear.Enabled
	ctx["custom_fields"] = cfs
	ctx["track_descriptions"] = tds
	ctx["pipeline_phases"] = phases
	ctx["agent"] = agent.Name
	ctx["commands"] = defaultCommands
	ctx["review_with_user"] = cfg.ReviewWithUser
	ctx["report_bugs"] = cfg.ReportBugs
	ctx["interview_with_docs"] = cfg.InterviewWithDocs

	return ctx
}
