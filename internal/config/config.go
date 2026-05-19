package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/agents"
)

const defaultSubprocessTimeout = 30 * time.Minute

// validate() rejects unparseable values at Load time, so this never returns an error.
func (p PipelineConfig) ResolvedSubprocessTimeout() time.Duration {
	if p.SubprocessTimeout == "" {
		return defaultSubprocessTimeout
	}
	d, err := time.ParseDuration(p.SubprocessTimeout)
	if err != nil || d <= 0 {
		return defaultSubprocessTimeout
	}
	return d
}

type CustomFieldDef struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"` // "string", "number", "boolean", "enum"
	Required   bool     `json:"required"`
	EnumValues []string `json:"enum_values,omitempty"`
}

type LinearConfig struct {
	Enabled   bool              `json:"enabled,omitempty"`
	APIKey    string            `json:"api_key,omitempty"`
	Team      string            `json:"team,omitempty"`
	ProjectID string            `json:"project_id,omitempty"`
	StatusMap map[string]string `json:"status_map,omitempty"`
}

type PipelineConfig struct {
	Skip           []string `json:"skip,omitempty"`
	BranchStrategy string   `json:"branch_strategy,omitempty"`
	// SubprocessTimeout caps each `claude` invocation — a hung sub-agent
	// otherwise blocks dispatch indefinitely. Empty falls back to defaultSubprocessTimeout.
	SubprocessTimeout string `json:"subprocess_timeout,omitempty"`
}

type HooksConfig struct {
	BeforeIssue string `json:"before_issue,omitempty"`
	AfterIssue  string `json:"after_issue,omitempty"`
	AfterBatch  string `json:"after_batch,omitempty"`
}

type IssueSchemaConfig struct {
	CustomFields []CustomFieldDef `json:"custom_fields,omitempty"`
}

// AgentOptions holds per-agent overrides for registry fields and arbitrary extra options.
// Known registry override fields are declared explicitly; all other keys are captured in Extra.
type AgentOptions struct {
	ProjectDir       *string        `json:"project_dir,omitempty"`
	UserDir          *string        `json:"user_dir,omitempty"`
	Scope            *string        `json:"scope,omitempty"`
	GitignorePattern *string        `json:"gitignore_pattern,omitempty"`
	Extra            map[string]any `json:"-"`
}

// AgentEntry is a bool|object union type for the agents config map.
// true = enabled with registry defaults, false = disabled (still validated),
// object = enabled with per-agent options.
type AgentEntry struct {
	Enabled bool
	Options AgentOptions
}

// MarshalJSON emits AgentEntry as a scalar bool when no options are set, or a
// mapping {enabled, ...options} when options exist. Keeps default configs
// minimal (`"agent-name": true`) while preserving the full form for overrides.
func (e AgentEntry) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if e.Options.ProjectDir != nil {
		out["project_dir"] = *e.Options.ProjectDir
	}
	if e.Options.UserDir != nil {
		out["user_dir"] = *e.Options.UserDir
	}
	if e.Options.Scope != nil {
		out["scope"] = *e.Options.Scope
	}
	if e.Options.GitignorePattern != nil {
		out["gitignore_pattern"] = *e.Options.GitignorePattern
	}
	for k, v := range e.Options.Extra {
		out[k] = v
	}
	if len(out) == 0 {
		return json.Marshal(e.Enabled)
	}
	out["enabled"] = e.Enabled
	return json.Marshal(out)
}

func (e *AgentEntry) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		e.Enabled = b
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("agents entry must be bool or object: %w", err)
	}

	getStr := func(key string) (*string, error) {
		v, ok := raw[key]
		if !ok {
			return nil, nil
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("decoding agent option %q: %w", key, err)
		}
		delete(raw, key)
		return &s, nil
	}

	enabled := true
	if v, ok := raw["enabled"]; ok {
		if err := json.Unmarshal(v, &enabled); err != nil {
			return fmt.Errorf("decoding agent enabled: %w", err)
		}
		delete(raw, "enabled")
	}

	pd, err := getStr("project_dir")
	if err != nil {
		return err
	}
	ud, err := getStr("user_dir")
	if err != nil {
		return err
	}
	sc, err := getStr("scope")
	if err != nil {
		return err
	}
	gp, err := getStr("gitignore_pattern")
	if err != nil {
		return err
	}

	var extra map[string]any
	if len(raw) > 0 {
		extra = make(map[string]any, len(raw))
		for k, v := range raw {
			var decoded any
			if err := json.Unmarshal(v, &decoded); err != nil {
				return fmt.Errorf("decoding agent option %q: %w", k, err)
			}
			extra[k] = decoded
		}
	}

	e.Enabled = enabled
	e.Options = AgentOptions{
		ProjectDir:       pd,
		UserDir:          ud,
		Scope:            sc,
		GitignorePattern: gp,
		Extra:            extra,
	}
	return nil
}

type ResolvedAgent struct {
	Name             string
	ProjectDir       string
	UserDir          string
	Scope            agents.Scope
	GitignorePattern string
	Extra            map[string]any
}

type Config struct {
	Tracks            []string        `json:"tracks"`
	WorkflowStates    []string        `json:"workflow_states"`
	PlansDir          string          `json:"plans_dir"`
	MaxPoints         int             `json:"max_points"`
	Agents            []ResolvedAgent `json:"agents"`
	rawAgents         map[string]*AgentEntry
	Pipeline          PipelineConfig    `json:"pipeline"`
	IssueSchema       IssueSchemaConfig `json:"issue_schema"`
	Linear            LinearConfig      `json:"linear"`
	Hooks             HooksConfig       `json:"hooks"`
	UpdateCheck       bool              `json:"update_check"`
	ManageGitignore   bool              `json:"manage_gitignore"`
	ReviewWithUser    bool              `json:"review_with_user"`
	ReportBugs        bool              `json:"report_bugs"`
	InterviewWithDocs bool              `json:"interview_with_docs"`
}

type PartialConfig struct {
	Tracks            []string               `json:"tracks,omitempty"`
	WorkflowStates    []string               `json:"workflow_states,omitempty"`
	PlansDir          *string                `json:"plans_dir,omitempty"`
	MaxPoints         *int                   `json:"max_points,omitempty"`
	Agents            map[string]*AgentEntry `json:"agents,omitempty"`
	Pipeline          *PipelineConfig        `json:"pipeline,omitempty"`
	IssueSchema       *IssueSchemaConfig     `json:"issue_schema,omitempty"`
	Linear            *LinearConfig          `json:"linear,omitempty"`
	Hooks             *HooksConfig           `json:"hooks,omitempty"`
	UpdateCheck       *bool                  `json:"update_check,omitempty"`
	ManageGitignore   *bool                  `json:"manage_gitignore,omitempty"`
	ReviewWithUser    *bool                  `json:"review_with_user,omitempty"`
	ReportBugs        *bool                  `json:"report_bugs,omitempty"`
	InterviewWithDocs *bool                  `json:"interview_with_docs,omitempty"`
}
