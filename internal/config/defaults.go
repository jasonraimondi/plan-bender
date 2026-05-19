package config

import "github.com/jasonraimondi/plan-bender/internal/agents"

func StarterConfig() PartialConfig {
	plansDir := "./.plan-bender/plans/"
	return PartialConfig{
		PlansDir: &plansDir,
		Agents: map[string]*AgentEntry{
			"claude-code": {Enabled: true},
			"pi":          {Enabled: true},
		},
	}
}

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
		Pipeline:          PipelineConfig{Skip: []string{}, BranchStrategy: "integration"},
		IssueSchema:       IssueSchemaConfig{CustomFields: []CustomFieldDef{}},
		Linear:            LinearConfig{},
		UpdateCheck:       true,
		ManageGitignore:   false,
		ReviewWithUser:    false,
		ReportBugs:        false,
		InterviewWithDocs: false,
	}
}
