package config

// StarterConfig returns a minimal PartialConfig for writing a new .plan-bender.yaml.
// Only includes fields worth surfacing to the user; everything else falls back to Defaults().
func StarterConfig() PartialConfig {
	plansDir := "./.plan-bender/plans/"
	maxPoints := 3
	return PartialConfig{
		PlansDir:  &plansDir,
		MaxPoints: &maxPoints,
		Agents:    []string{"claude-code"},
	}
}

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
