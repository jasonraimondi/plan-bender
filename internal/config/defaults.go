package config

// Defaults returns the base configuration with TS-identical default values.
func Defaults() Config {
	return Config{
		Tracks:         []string{"intent", "experience", "data", "rules", "resilience"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"},
		PlansDir:       "./.plan-bender/plans/",
		MaxPoints:      3,
		Agents:         []string{"claude-code"},
		Pipeline:       PipelineConfig{Skip: []string{}},
		IssueSchema:    IssueSchemaConfig{CustomFields: []CustomFieldDef{}},
		Linear:         LinearConfig{},
		UpdateCheck:    true,
	}
}
