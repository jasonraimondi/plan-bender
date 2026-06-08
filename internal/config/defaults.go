package config

import "github.com/jasonraimondi/plan-bender/internal/agents"

// SchemaURL is the published JSON Schema for .plan-bender.json. Scaffolded
// configs reference it via "$schema" so editors offer validation/autocomplete.
const SchemaURL = "https://raw.githubusercontent.com/jasonraimondi/plan-bender/main/schema/plan-bender.schema.json"

func StarterConfig() PartialConfig {
	plansDir := "./.plan-bender/plans/"
	return PartialConfig{
		Schema:   SchemaURL,
		PlansDir: &plansDir,
		Agents: map[string]*AgentEntry{
			"claude-code": {Enabled: true},
		},
	}
}

func Defaults() Config {
	ac, _ := agents.Get("claude-code")
	return Config{
		Tracks:         []string{"intent", "experience", "data", "rules", "resilience"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "needs-input", "in-review", "qa", "done", "canceled"},
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
		Pipeline:          PipelineConfig{Skip: []string{}, BranchStrategy: "integration"},
		IssueSchema:       IssueSchemaConfig{CustomFields: []CustomFieldDef{}},
		Linear:            LinearConfig{},
		UpdateCheck:       true,
		ManageGitignore:   false,
		ReviewWithUser:    false,
		ReportBugs:        false,
		InterviewWithDocs: false,
		NoImplement:       false,
	}
}
