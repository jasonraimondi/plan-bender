package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewMigrateCmd creates the migrate command. One-shot: convert legacy
// .yaml plan and config files to .json on disk, then delete the originals.
// Existing .json siblings are left untouched.
func NewMigrateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Convert legacy YAML plan/config files to JSON",
		Long: `Walks .plan-bender/plans/*/prd.yaml and issues/*.yaml plus the project
and local config files, rewrites them as .json, and deletes the originals.
Existing .json siblings are skipped — migrate is idempotent and safe to re-run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			out := cmd.OutOrStdout()

			// Config files: ~/.config/plan-bender/defaults.yaml, .plan-bender.yaml, .plan-bender.local.yaml
			home, _ := os.UserHomeDir()
			configPaths := []string{}
			if home != "" {
				configPaths = append(configPaths, filepath.Join(home, ".config", "plan-bender", "defaults.yaml"))
			}
			configPaths = append(configPaths,
				filepath.Join(root, ".plan-bender.yaml"),
				filepath.Join(root, ".plan-bender.local.yaml"),
			)

			converted := 0
			skipped := 0
			for _, p := range configPaths {
				did, err := migrateOne(p, dryRun, out)
				if err != nil {
					return fmt.Errorf("migrate %s: %w", p, err)
				}
				if did {
					converted++
				} else if exists(p) {
					skipped++
				}
			}

			cfg, err := config.Load(root)
			if err == nil {
				plansDir := cfg.PlansDir
				if !filepath.IsAbs(plansDir) {
					plansDir = filepath.Join(root, plansDir)
				}
				err := filepath.WalkDir(plansDir, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return nil // best-effort walk; surface aggregate issues below
					}
					if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
						return nil
					}
					did, mErr := migrateOne(path, dryRun, out)
					if mErr != nil {
						return fmt.Errorf("migrate %s: %w", path, mErr)
					}
					if did {
						converted++
					} else {
						skipped++
					}
					return nil
				})
				if err != nil {
					return err
				}
			}

			if dryRun {
				fmt.Fprintf(out, "dry-run: %d to convert, %d skipped\n", converted, skipped)
			} else {
				fmt.Fprintf(out, "converted %d files (%d skipped — already had .json sibling)\n", converted, skipped)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what would change without writing")
	return cmd
}

// migrateOne converts a single .yaml file to .json. Returns true when a
// conversion happened. Skips and returns (false, nil) when:
//   - the source does not exist
//   - a .json sibling already exists (idempotent re-run)
func migrateOne(yamlPath string, dryRun bool, out interface{ Write(p []byte) (int, error) }) (bool, error) {
	if !exists(yamlPath) {
		return false, nil
	}
	jsonPath := strings.TrimSuffix(yamlPath, ".yaml") + ".json"
	if exists(jsonPath) {
		fmt.Fprintf(out, "skip %s — %s already exists\n", yamlPath, filepath.Base(jsonPath))
		return false, nil
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return false, fmt.Errorf("reading: %w", err)
	}

	encoded, err := encodeYAMLToJSON(yamlPath, data)
	if err != nil {
		return false, err
	}

	if dryRun {
		fmt.Fprintf(out, "would convert %s → %s\n", yamlPath, jsonPath)
		return true, nil
	}

	if err := os.WriteFile(jsonPath, encoded, 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", jsonPath, err)
	}
	if err := os.Remove(yamlPath); err != nil {
		return false, fmt.Errorf("removing %s: %w", yamlPath, err)
	}
	fmt.Fprintf(out, "converted %s → %s\n", yamlPath, jsonPath)
	return true, nil
}

// encodeYAMLToJSON dispatches by path: plan files (prd.yaml, issues/*.yaml)
// flow through the typed schema decoder so bare-colon list items collapse to
// strings via proseList; config files use the raw any-walk path. Mixing them
// matters because the typed structs only describe plan files — a config file
// would lose unknown keys if forced through them.
func encodeYAMLToJSON(yamlPath string, data []byte) ([]byte, error) {
	switch {
	case filepath.Base(yamlPath) == "prd.yaml":
		return encodePRDYAML(data)
	case filepath.Base(filepath.Dir(yamlPath)) == "issues":
		return encodeIssueYAML(data)
	default:
		return encodeRawYAML(data)
	}
}

func encodeRawYAML(data []byte) ([]byte, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	raw = normalizeForJSON(raw)
	return marshalIndentedJSON(raw)
}

func encodePRDYAML(data []byte) ([]byte, error) {
	var doc prdYAMLDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml decode prd: %w", err)
	}
	return marshalIndentedJSON(doc.toSchema())
}

func encodeIssueYAML(data []byte) ([]byte, error) {
	var doc issueYAMLDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml decode issue: %w", err)
	}
	return marshalIndentedJSON(doc.toSchema())
}

func marshalIndentedJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("json encode: %w", err)
	}
	return buf.Bytes(), nil
}

// normalizeForJSON converts map[any]any (yaml.v3's default for mappings) into
// map[string]any so encoding/json can marshal it.
func normalizeForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normalizeForJSON(val)
		}
		return m
	case map[string]any:
		for k, val := range t {
			t[k] = normalizeForJSON(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = normalizeForJSON(val)
		}
		return t
	}
	return v
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// proseList is a migration-only []string that tolerates YAML list items
// written as `- some prose: more prose` without surrounding quotes. yaml.v3
// parses such items as a single-key mapping; without flattening, the post-
// migration strict JSON decoder rejects the file. Production code never sees
// this type — once on disk, the format is plain []string in JSON.
type proseList []string

// flattenMaxDepth bounds recursive descent so circular YAML anchors cannot
// exhaust the goroutine stack.
const flattenMaxDepth = 32

func (s *proseList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Tag == "!!null" {
		*s = nil
		return nil
	}
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: expected a list, got node kind %d", value.Line, value.Kind)
	}
	out := make([]string, 0, len(value.Content))
	for _, item := range value.Content {
		if isYAMLNull(item) {
			return fmt.Errorf("line %d: null list item not allowed", item.Line)
		}
		v, err := flattenProseItem(item, 0)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	*s = out
	return nil
}

func flattenProseItem(n *yaml.Node, depth int) (string, error) {
	if depth > flattenMaxDepth {
		return "", fmt.Errorf("line %d: list item nested too deeply", n.Line)
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value, nil
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := flattenProseItem(n.Content[i], depth+1)
			if err != nil {
				return "", err
			}
			if isYAMLNull(n.Content[i+1]) {
				parts = append(parts, k)
				continue
			}
			v, err := flattenProseItem(n.Content[i+1], depth+1)
			if err != nil {
				return "", err
			}
			if v == "" {
				parts = append(parts, k)
			} else {
				parts = append(parts, k+": "+v)
			}
		}
		return strings.Join(parts, ", "), nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			s, err := flattenProseItem(c, depth+1)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ", "), nil
	case yaml.AliasNode:
		if n.Alias == nil {
			return "", fmt.Errorf("line %d: nil alias target", n.Line)
		}
		return flattenProseItem(n.Alias, depth+1)
	}
	return "", fmt.Errorf("line %d: unsupported list item kind", n.Line)
}

func isYAMLNull(n *yaml.Node) bool {
	return n != nil && n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// prdYAMLDoc mirrors schema.PrdYaml with proseList fields for YAML decode.
// JSON output matches schema.PrdYaml because toSchema converts every field
// before encoding — the type lives here only to absorb bare-colon items.
type prdYAMLDoc struct {
	Name          string             `yaml:"name"`
	Slug          string             `yaml:"slug"`
	Status        string             `yaml:"status"`
	Created       string             `yaml:"created"`
	Updated       string             `yaml:"updated"`
	Description   string             `yaml:"description"`
	Why           string             `yaml:"why"`
	Outcome       string             `yaml:"outcome"`
	InScope       proseList          `yaml:"in_scope,omitempty"`
	OutOfScope    proseList          `yaml:"out_of_scope,omitempty"`
	UseCases      []schema.UseCase   `yaml:"use_cases,omitempty"`
	Decisions     proseList          `yaml:"decisions,omitempty"`
	OpenQuestions proseList          `yaml:"open_questions,omitempty"`
	Risks         proseList          `yaml:"risks,omitempty"`
	Validation    proseList          `yaml:"validation,omitempty"`
	Notes         *string            `yaml:"notes,omitempty"`
	DevCommand    *string            `yaml:"dev_command,omitempty"`
	BaseURL       *string            `yaml:"base_url,omitempty"`
	Linear        *schema.LinearRef  `yaml:"linear,omitempty"`
}

func (p *prdYAMLDoc) toSchema() *schema.PrdYaml {
	return &schema.PrdYaml{
		Name:          p.Name,
		Slug:          p.Slug,
		Status:        p.Status,
		Created:       p.Created,
		Updated:       p.Updated,
		Description:   p.Description,
		Why:           p.Why,
		Outcome:       p.Outcome,
		InScope:       []string(p.InScope),
		OutOfScope:    []string(p.OutOfScope),
		UseCases:      p.UseCases,
		Decisions:     []string(p.Decisions),
		OpenQuestions: []string(p.OpenQuestions),
		Risks:         []string(p.Risks),
		Validation:    []string(p.Validation),
		Notes:         p.Notes,
		DevCommand:    p.DevCommand,
		BaseURL:       p.BaseURL,
		Linear:        p.Linear,
	}
}

// issueYAMLDoc mirrors schema.IssueYaml with proseList fields for YAML decode.
type issueYAMLDoc struct {
	ID                 int       `yaml:"id"`
	Slug               string    `yaml:"slug"`
	Name               string    `yaml:"name"`
	Track              string    `yaml:"track"`
	Status             string    `yaml:"status"`
	Priority           string    `yaml:"priority"`
	Points             int       `yaml:"points"`
	Labels             []string  `yaml:"labels"`
	Assignee           *string   `yaml:"assignee"`
	BlockedBy          []int     `yaml:"blocked_by"`
	Blocking           []int     `yaml:"blocking"`
	Branch             *string   `yaml:"branch"`
	PR                 *string   `yaml:"pr"`
	LinearID           *string   `yaml:"linear_id"`
	LinearURL          string    `yaml:"linear_url,omitempty"`
	Created            string    `yaml:"created"`
	Updated            string    `yaml:"updated"`
	TDD                bool      `yaml:"tdd"`
	Headed             *bool     `yaml:"headed,omitempty"`
	Outcome            string    `yaml:"outcome"`
	Scope              string    `yaml:"scope"`
	AcceptanceCriteria proseList `yaml:"acceptance_criteria"`
	Steps              proseList `yaml:"steps"`
	UseCases           proseList `yaml:"use_cases"`
	Notes              *string   `yaml:"notes,omitempty"`
}

func (i *issueYAMLDoc) toSchema() *schema.IssueYaml {
	return &schema.IssueYaml{
		ID:                 i.ID,
		Slug:               i.Slug,
		Name:               i.Name,
		Track:              i.Track,
		Status:             i.Status,
		Priority:           i.Priority,
		Points:             i.Points,
		Labels:             i.Labels,
		Assignee:           i.Assignee,
		BlockedBy:          i.BlockedBy,
		Blocking:           i.Blocking,
		Branch:             i.Branch,
		PR:                 i.PR,
		LinearID:           i.LinearID,
		LinearURL:          i.LinearURL,
		Created:            i.Created,
		Updated:            i.Updated,
		TDD:                i.TDD,
		Headed:             i.Headed,
		Outcome:            i.Outcome,
		Scope:              i.Scope,
		AcceptanceCriteria: []string(i.AcceptanceCriteria),
		Steps:              []string(i.Steps),
		UseCases:           []string(i.UseCases),
		Notes:              i.Notes,
	}
}
