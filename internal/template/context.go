package template

import "github.com/jasonraimondi/plan-bender/internal/config"

var defaultTrackDescriptions = map[string]string{
	"intent":     "What the system should do — features, commands, API behavior",
	"experience": "How users interact with the system — UI, UX, accessibility",
	"data":       "Data models, schemas, storage, migrations, CRUD behavior",
	"rules":      "Business rules, validation, authorization, constraints",
	"resilience": "Error handling, retries, fallbacks, monitoring, recovery",
}

// PipelinePhase is a step in the planning pipeline.
type PipelinePhase struct {
	Name        string
	Description string
	Skill       string
}

var defaultPipelinePhases = []PipelinePhase{
	{Name: "Interview", Description: "Stress-test your plan", Skill: "bender-interview-me"},
	{Name: "Write PRD", Description: "Create a PRD", Skill: "bender-write-prd"},
	{Name: "PRD to Issues", Description: "Break PRD into issues", Skill: "bender-prd-to-issues"},
	{Name: "Write Issue", Description: "Create a single issue", Skill: "bender-write-issue"},
	{Name: "Review PRD", Description: "Review plan quality", Skill: "bender-review-prd"},
	{Name: "Implement PRD", Description: "Work through issues", Skill: "bender-implement-prd"},
	{Name: "Implement Issue", Description: "Implement one issue", Skill: "bender-implement-issue"},
}

// BuildContext produces the template rendering context from config for a specific agent.
func BuildContext(cfg config.Config, agent string) map[string]any {
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
		if !skipSet[p.Skill] {
			phases = append(phases, map[string]string{
				"name":        p.Name,
				"description": p.Description,
				"skill":       p.Skill,
			})
		}
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

	return map[string]any{
		"plans_dir":          cfg.PlansDir,
		"tracks":             cfg.Tracks,
		"workflow_states":    cfg.WorkflowStates,
		"step_pattern":       cfg.StepPattern,
		"max_points":         cfg.MaxPoints,
		"has_backend_sync":   cfg.Linear.Enabled,
		"custom_fields":      cfs,
		"track_descriptions": tds,
		"pipeline_phases":    phases,
		"agent":              agent,
	}
}
