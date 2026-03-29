package config

// Defaults returns the base configuration with TS-identical default values.
func Defaults() Config {
	return Config{
		Backend:        BackendYAMLFS,
		Tracks:         []string{"intent", "experience", "data", "rules", "resilience"},
		WorkflowStates: []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"},
		StepPattern:    "Target — behavior",
		PlansDir:       "./.plan-bender/plans/",
		MaxPoints:      3,
		Pipeline:       PipelineConfig{Skip: []string{}},
		IssueSchema:    IssueSchemaConfig{CustomFields: []CustomFieldDef{}},
		Linear:         LinearConfig{},
		InstallTarget:  InstallTargetProject,
		UpdateCheck:    true,
	}
}
