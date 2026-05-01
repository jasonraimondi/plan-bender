package config

import (
	"fmt"
	"sort"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/agents"
)

func validate(cfg *Config) error {
	var errs []FieldError

	if len(cfg.Tracks) == 0 {
		errs = append(errs, FieldError{Field: "tracks", Message: "must not be empty"})
	}

	if cfg.MaxPoints < 1 {
		errs = append(errs, FieldError{Field: "max_points", Message: "must be at least 1"})
	}

	resolved, agentErrs := resolveAgents(cfg.rawAgents)
	errs = append(errs, agentErrs...)
	if len(agentErrs) == 0 {
		if len(resolved) == 0 {
			errs = append(errs, FieldError{Field: "agents", Message: "must have at least one enabled agent"})
		} else {
			cfg.Agents = resolved
		}
	}

	if cfg.Pipeline.BranchStrategy != "" && cfg.Pipeline.BranchStrategy != "integration" && cfg.Pipeline.BranchStrategy != "direct" {
		errs = append(errs, FieldError{
			Field:   "pipeline.branch_strategy",
			Message: fmt.Sprintf("must be 'integration' or 'direct', got %q", cfg.Pipeline.BranchStrategy),
		})
	}

	if cfg.Pipeline.SubprocessTimeout != "" {
		if d, err := time.ParseDuration(cfg.Pipeline.SubprocessTimeout); err != nil {
			errs = append(errs, FieldError{
				Field:   "pipeline.subprocess_timeout",
				Message: fmt.Sprintf("must be a Go duration string (e.g. \"30m\"), got %q: %v", cfg.Pipeline.SubprocessTimeout, err),
			})
		} else if d <= 0 {
			errs = append(errs, FieldError{
				Field:   "pipeline.subprocess_timeout",
				Message: fmt.Sprintf("must be positive, got %q", cfg.Pipeline.SubprocessTimeout),
			})
		}
	}

	if cfg.Linear.Enabled {
		if cfg.Linear.APIKey == "" {
			errs = append(errs, FieldError{Field: "linear.api_key", Message: "required when linear is enabled"})
		}
		if cfg.Linear.Team == "" {
			errs = append(errs, FieldError{Field: "linear.team", Message: "required when linear is enabled"})
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

// resolveAgents validates all agent names against the registry (including disabled entries)
// and returns the slice of enabled agents with registry defaults merged with per-agent overrides.
// Returns field errors for unknown agent names.
func resolveAgents(rawAgents map[string]*AgentEntry) ([]ResolvedAgent, []FieldError) {
	if len(rawAgents) == 0 {
		return nil, nil
	}

	// Sort for deterministic output
	names := make([]string, 0, len(rawAgents))
	for name := range rawAgents {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []FieldError
	var resolved []ResolvedAgent

	for _, name := range names {
		entry := rawAgents[name]
		ac, err := agents.Get(name)
		if err != nil {
			errs = append(errs, FieldError{
				Field:   fmt.Sprintf("agents[%s]", name),
				Message: fmt.Sprintf("unknown agent %q", name),
			})
			continue
		}

		// Disabled agents are validated but not included in the resolved slice
		if entry == nil || !entry.Enabled {
			continue
		}

		ra := ResolvedAgent{
			Name:             name,
			ProjectDir:       ac.ProjectDir,
			UserDir:          ac.UserDir,
			Scope:            ac.Scope,
			GitignorePattern: ac.GitignorePattern,
			Extra:            entry.Options.Extra,
		}

		// Apply per-agent overrides over registry defaults
		if entry.Options.ProjectDir != nil {
			ra.ProjectDir = *entry.Options.ProjectDir
		}
		if entry.Options.UserDir != nil {
			ra.UserDir = *entry.Options.UserDir
		}
		if entry.Options.Scope != nil {
			ra.Scope = agents.Scope(*entry.Options.Scope)
		}
		if entry.Options.GitignorePattern != nil {
			ra.GitignorePattern = *entry.Options.GitignorePattern
		}

		resolved = append(resolved, ra)
	}

	return resolved, errs
}
