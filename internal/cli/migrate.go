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

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("yaml decode: %w", err)
	}
	raw = normalizeForJSON(raw)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(raw); err != nil {
		return false, fmt.Errorf("json encode: %w", err)
	}

	if dryRun {
		fmt.Fprintf(out, "would convert %s → %s\n", yamlPath, jsonPath)
		return true, nil
	}

	if err := os.WriteFile(jsonPath, buf.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", jsonPath, err)
	}
	if err := os.Remove(yamlPath); err != nil {
		return false, fmt.Errorf("removing %s: %w", yamlPath, err)
	}
	fmt.Fprintf(out, "converted %s → %s\n", yamlPath, jsonPath)
	return true, nil
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
