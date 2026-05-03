package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
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

			plans := planrepo.NewProd(cfg.PlansDir)
			sess, err := plans.OpenOrCreate(slug)
			if err != nil {
				return err
			}
			defer sess.Close()

			if err := stageIssue(sess, issue); err != nil {
				return err
			}
			if err := sess.Commit(cfg); err != nil {
				return reportCommitError(cmd, err)
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

// stageIssue routes the write through CreateIssue when the ID is new, or
// UpdateIssue when it already exists in the snapshot. Keeps the command's
// upsert-style behavior intact while satisfying the session's separate
// create / update entry points.
func stageIssue(sess *planrepo.PlanSession, issue schema.IssueYaml) error {
	snap, err := sess.Snapshot()
	if err != nil {
		return err
	}
	for _, existing := range snap.Issues {
		if existing.ID == issue.ID {
			return sess.UpdateIssue(issue)
		}
	}
	return sess.CreateIssue(issue)
}
