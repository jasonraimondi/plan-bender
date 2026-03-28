package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewWriteIssueCmd creates the write-issue command.
func NewWriteIssueCmd() *cobra.Command {
	var slug string

	cmd := &cobra.Command{
		Use:   "write-issue [file]",
		Short: "Validate and write an issue YAML file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			data, err := readInput(cmd, args)
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

			if slug == "" {
				return fmt.Errorf("--slug is required")
			}

			dir := filepath.Join(cfg.PlansDir, slug, "issues")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}

			filename := fmt.Sprintf("%d-%s.yaml", issue.ID, issue.Slug)
			outPath := filepath.Join(dir, filename)
			outData, err := yaml.Marshal(&issue)
			if err != nil {
				return err
			}

			if err := atomicWriteFile(outPath, outData, 0o644); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&slug, "slug", "", "project slug (required)")
	return cmd
}
