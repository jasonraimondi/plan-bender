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
)

// NewWriteIssueCmd creates the write-issue command.
func NewWriteIssueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "write-issue <slug> [file]",
		Short: "Validate and write an issue JSON file",
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

			var issue schema.Issue
			if err := planrepo.StrictUnmarshal(data, &issue); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
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

			found := false
			for _, existing := range sess.Snapshot().Issues {
				if existing.ID == issue.ID {
					found = true
					break
				}
			}
			if found {
				if err := sess.UpdateIssue(issue); err != nil {
					return err
				}
			} else if err := sess.CreateIssue(issue); err != nil {
				return err
			}
			if err := sess.Commit(cfg); err != nil {
				return reportCommitError(cmd, err)
			}

			outPath := filepath.Join(cfg.PlansDir, slug, "issues",
				fmt.Sprintf("%d-%s.json", issue.ID, issue.Slug))
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
