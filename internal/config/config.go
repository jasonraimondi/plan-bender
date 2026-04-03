package config

import (
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/agents"
	"gopkg.in/yaml.v3"
)

// CustomFieldDef defines a custom field on issue YAML.
type CustomFieldDef struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"` // "string", "number", "boolean", "enum"
	Required   bool     `yaml:"required"`
	EnumValues []string `yaml:"enum_values,omitempty"`
}

// LinearConfig holds Linear integration settings.
type LinearConfig struct {
	Enabled   bool              `yaml:"enabled,omitempty"`
	APIKey    string            `yaml:"api_key,omitempty"`
	Team      string            `yaml:"team,omitempty"`
	ProjectID string            `yaml:"project_id,omitempty"`
	StatusMap map[string]string `yaml:"status_map,omitempty"`
}

// PipelineConfig controls which pipeline steps to skip.
type PipelineConfig struct {
	Skip []string `yaml:"skip,omitempty"`
}

// IssueSchemaConfig controls custom fields on issues.
type IssueSchemaConfig struct {
	CustomFields []CustomFieldDef `yaml:"custom_fields,omitempty"`
}

// AgentOptions holds per-agent overrides for registry fields and arbitrary extra options.
// Known registry override fields are declared explicitly; all other keys are captured in Extra.
type AgentOptions struct {
	ProjectDir       *string        `yaml:"project_dir,omitempty"`
	UserDir          *string        `yaml:"user_dir,omitempty"`
	Scope            *string        `yaml:"scope,omitempty"`
	GitignorePattern *string        `yaml:"gitignore_pattern,omitempty"`
	Extra            map[string]any `yaml:",inline"`
}

// AgentEntry is a bool|object union type for the agents config map.
// true = enabled with registry defaults, false = disabled (still validated),
// object = enabled with per-agent options.
type AgentEntry struct {
	Enabled bool
	Options AgentOptions
}

// UnmarshalYAML implements a custom YAML unmarshaler that handles bool and object values.
func (e *AgentEntry) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var b bool
		if err := value.Decode(&b); err != nil {
			return fmt.Errorf("agents entry must be bool or object: %w", err)
		}
		e.Enabled = b
		return nil
	case yaml.MappingNode:
		if err := value.Decode(&e.Options); err != nil {
			return fmt.Errorf("decoding agent options: %w", err)
		}
		e.Enabled = true
		return nil
	default:
		return fmt.Errorf("agents entry must be bool or object, got YAML tag %q", value.Tag)
	}
}

// ResolvedAgent is a fully resolved agent configuration with registry defaults merged
// with any per-agent overrides. Only enabled agents appear as ResolvedAgent values.
type ResolvedAgent struct {
	Name             string
	ProjectDir       string
	UserDir          string
	Scope            agents.Scope
	GitignorePattern string
	Extra            map[string]any
}

// Config is the fully resolved configuration.
type Config struct {
	Tracks         []string          `yaml:"tracks"`
	WorkflowStates []string          `yaml:"workflow_states"`
	PlansDir       string            `yaml:"plans_dir"`
	MaxPoints      int               `yaml:"max_points"`
	Agents         []ResolvedAgent   `yaml:"agents"`
	rawAgents      map[string]*AgentEntry
	Pipeline       PipelineConfig    `yaml:"pipeline"`
	IssueSchema    IssueSchemaConfig `yaml:"issue_schema"`
	Linear         LinearConfig      `yaml:"linear"`
	UpdateCheck    bool              `yaml:"update_check"`
}

// PartialConfig is used for YAML layer loading — all fields optional.
type PartialConfig struct {
	Tracks         []string                `yaml:"tracks,omitempty"`
	WorkflowStates []string                `yaml:"workflow_states,omitempty"`
	PlansDir       *string                 `yaml:"plans_dir,omitempty"`
	MaxPoints      *int                    `yaml:"max_points,omitempty"`
	Agents         map[string]*AgentEntry  `yaml:"agents,omitempty"`
	Pipeline       *PipelineConfig         `yaml:"pipeline,omitempty"`
	IssueSchema    *IssueSchemaConfig      `yaml:"issue_schema,omitempty"`
	Linear         *LinearConfig           `yaml:"linear,omitempty"`
	UpdateCheck    *bool                   `yaml:"update_check,omitempty"`
}
