package config

import "github.com/jasonraimondi/plan-bender/internal/agents"

// StarterConfig returns a minimal PartialConfig for writing a new .plan-bender.yaml.
// Only includes fields worth surfacing to the user; everything else falls back to Defaults().
func StarterConfig() PartialConfig {
	plansDir := "./.plan-bender/plans/"
	maxPoints := 3
	return PartialConfig{
		PlansDir:  &plansDir,
		MaxPoints: &maxPoints,
		Agents:    map[string]*AgentEntry{"claude-code": {Enabled: true}},
	}
}

// Defaults returns the base configuration with default values.
func Defaults() Config {
	ac, _ := agents.Get("claude-code")
	return Config{
		Tracks:         []string{"intent", "experience", "data", "rules", "resilience"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"},
		PlansDir:       "./.plan-bender/plans/",
		MaxPoints:      3,
		rawAgents:      map[string]*AgentEntry{"claude-code": {Enabled: true}},
		Agents: []ResolvedAgent{{
			Name:             ac.Name,
			ProjectDir:       ac.ProjectDir,
			UserDir:          ac.UserDir,
			Scope:            ac.Scope,
			GitignorePattern: ac.GitignorePattern,
		}},
		Pipeline:        PipelineConfig{Skip: []string{}, BranchStrategy: "integration"},
		IssueSchema:     IssueSchemaConfig{CustomFields: []CustomFieldDef{}},
		Linear:          LinearConfig{},
		UpdateCheck:     true,
		ManageGitignore: true,
		ReviewWithUser:  []string{"bender-write-prd", "bender-write-issue"},
	}
}
