package config

import (
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/agents"
)

func validate(cfg *Config) error {
	var errs []FieldError

	if cfg.Backend != BackendYAMLFS && cfg.Backend != BackendLinear {
		errs = append(errs, FieldError{Field: "backend", Message: fmt.Sprintf("invalid backend %q", cfg.Backend)})
	}

	if len(cfg.Tracks) == 0 {
		errs = append(errs, FieldError{Field: "tracks", Message: "must not be empty"})
	}

	if cfg.MaxPoints < 1 {
		errs = append(errs, FieldError{Field: "max_points", Message: "must be at least 1"})
	}

	// Deduplicate agents
	seen := make(map[string]bool, len(cfg.Agents))
	deduped := make([]string, 0, len(cfg.Agents))
	for _, a := range cfg.Agents {
		if !seen[a] {
			seen[a] = true
			deduped = append(deduped, a)
		}
	}
	cfg.Agents = deduped

	if len(cfg.Agents) == 0 {
		errs = append(errs, FieldError{Field: "agents", Message: "must not be empty"})
	}
	for i, name := range cfg.Agents {
		if _, err := agents.Get(name); err != nil {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("agents[%d]", i),
				Message: fmt.Sprintf("unknown agent %q", name),
			})
		}
	}

	if cfg.Backend == BackendLinear {
		if cfg.Linear.APIKey == "" {
			errs = append(errs, FieldError{Field: "linear.api_key", Message: "required when backend is linear"})
		}
		if cfg.Linear.Team == "" {
			errs = append(errs, FieldError{Field: "linear.team", Message: "required when backend is linear"})
		}
	}

	for i, cf := range cfg.IssueSchema.CustomFields {
		if cf.Name == "" {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("issue_schema.custom_fields[%d].name", i),
				Message: "must not be empty",
			})
		}
		if cf.Type == "enum" && len(cf.EnumValues) == 0 {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("issue_schema.custom_fields[%d].enum_values", i),
				Message: "required when type is enum",
			})
		}
	}

	if len(errs) > 0 {
		return &ConfigError{Errors: errs}
	}
	return nil
}
