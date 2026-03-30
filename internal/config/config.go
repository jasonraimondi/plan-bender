package config

// Backend is the tracking backend type.
type Backend string

const (
	BackendYAMLFS Backend = "yaml-fs"
	BackendLinear Backend = "linear"
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
	APIKey    string            `yaml:"api_key,omitempty"`
	Team      string            `yaml:"team,omitempty"`
	ProjectID string            `yaml:"project_id,omitempty"`
	StatusMap map[string]string `yaml:"status_map,omitempty"`
}

// PipelineConfig controls which pipeline steps to skip.
type PipelineConfig struct {
	Skip []string `yaml:"skip"`
}

// IssueSchemaConfig controls custom fields on issues.
type IssueSchemaConfig struct {
	CustomFields []CustomFieldDef `yaml:"custom_fields"`
}

// Config is the fully resolved configuration.
type Config struct {
	Backend        Backend           `yaml:"backend"`
	Tracks         []string          `yaml:"tracks"`
	WorkflowStates []string          `yaml:"workflow_states"`
	StepPattern    string            `yaml:"step_pattern"`
	PlansDir       string            `yaml:"plans_dir"`
	MaxPoints      int               `yaml:"max_points"`
	Agents         []string          `yaml:"agents"`
	Pipeline       PipelineConfig    `yaml:"pipeline"`
	IssueSchema    IssueSchemaConfig `yaml:"issue_schema"`
	Linear         LinearConfig      `yaml:"linear"`
	Agents         []string          `yaml:"agents"`
	UpdateCheck    bool              `yaml:"update_check"`
}

// PartialConfig is used for YAML layer loading — all fields optional.
type PartialConfig struct {
	Backend        *Backend           `yaml:"backend,omitempty"`
	Tracks         []string           `yaml:"tracks,omitempty"`
	WorkflowStates []string           `yaml:"workflow_states,omitempty"`
	StepPattern    *string            `yaml:"step_pattern,omitempty"`
	PlansDir       *string            `yaml:"plans_dir,omitempty"`
	MaxPoints      *int               `yaml:"max_points,omitempty"`
	Agents         []string           `yaml:"agents,omitempty"`
	Pipeline       *PipelineConfig    `yaml:"pipeline,omitempty"`
	IssueSchema    *IssueSchemaConfig `yaml:"issue_schema,omitempty"`
	Linear         *LinearConfig      `yaml:"linear,omitempty"`
	Agents         []string           `yaml:"agents,omitempty"`
	UpdateCheck    *bool              `yaml:"update_check,omitempty"`
}
