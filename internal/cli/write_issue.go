package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewWriteIssueCmd creates the write-issue command.
func NewWriteIssueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "write-issue <slug> [file]",
		Short: "Validate and write an issue YAML file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			slug := args[0]

			data, err := readInput(cmd, args[1:])
			if err != nil {
				return err
			}

			var issue schema.IssueYaml
			if err := yaml.Unmarshal(data, &issue); err != nil {
				return fmt.Errorf("invalid YAML: %w", err)
			}

			errs := issue.Validate(cfg)
			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", e)
				}
				return fmt.Errorf("validation failed")
			}

			store := backend.NewProdPlanStore(cfg.PlansDir)
			if err := store.WriteIssue(slug, &issue); err != nil {
				return err
			}

			outPath := filepath.Join(cfg.PlansDir, slug, "issues",
				fmt.Sprintf("%d-%s.yaml", issue.ID, issue.Slug))
			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"status": "ok",
					"file":   outPath,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}
}
