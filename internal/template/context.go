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

var defaultPipelinePhases = []PipelinePhase{
	{Name: "Interview", Description: "Stress-test your plan", Skill: "bender-interview-me"},
	{Name: "Write PRD", Description: "Create a PRD", Skill: "bender-write-prd"},
	{Name: "PRD to Issues", Description: "Break PRD into issues", Skill: "bender-prd-to-issues"},
	{Name: "Write Issue", Description: "Create a single issue", Skill: "bender-write-issue"},
	{Name: "Review PRD", Description: "Review plan quality", Skill: "bender-review-prd"},
	{Name: "Implement PRD", Description: "Work through issues", Skill: "bender-implement-prd"},
	{Name: "Implement Issue", Description: "Implement one issue", Skill: "bender-implement-issue"},
	{Name: "Sync with Linear", Description: "Push local plan to Linear or pull Linear state", Skill: "bender-sync-linear", RequiresBackend: true},
}

// BuildContext produces the template rendering context from config for a specific agent.
// Extra keys from the agent are flat-merged first; built-in keys always win on collision.
func BuildContext(cfg config.Config, agent config.ResolvedAgent) map[string]any {
	ctx := make(map[string]any)

	// Flat-merge agent extra options first so built-ins can override on collision
	for k, v := range agent.Extra {
		ctx[k] = v
	}

	// Track descriptions
	tds := make([]map[string]string, len(cfg.Tracks))
	for i, t := range cfg.Tracks {
		desc, ok := defaultTrackDescriptions[t]
		if !ok {
			desc = t + " track"
		}
		tds[i] = map[string]string{"name": t, "description": desc}
	}

	// Pipeline phases (filter out skipped)
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
		phases = append(phases, map[string]string{
			"name":        p.Name,
			"description": p.Description,
			"skill":       p.Skill,
		})
	}

	// Custom fields
	cfs := make([]map[string]any, len(cfg.IssueSchema.CustomFields))
	for i, f := range cfg.IssueSchema.CustomFields {
		cfs[i] = map[string]any{
			"name":        f.Name,
			"type":        f.Type,
			"required":    f.Required,
			"enum_values": f.EnumValues,
		}
	}

	// Built-in keys overwrite any extra keys with the same name
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

	return ctx
}
