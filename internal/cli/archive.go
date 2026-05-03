package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewArchiveCmd creates the archive command.
func NewArchiveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "archive <slug>",
		Short: "Archive a completed plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			planDir := filepath.Join(cfg.PlansDir, slug)
			issues, err := readIssuesForArchive(cfg.PlansDir, slug)
			if err != nil {
				return err
			}

			// Check for active issues
			if !force {
				var active []string
				for _, iss := range issues {
					if iss.Status == "in-progress" || iss.Status == "blocked" {
						active = append(active, fmt.Sprintf("#%d (%s)", iss.ID, iss.Status))
					}
				}
				if len(active) > 0 {
					return fmt.Errorf("active issues: %s — use --force to override", strings.Join(active, ", "))
				}
			}

			// Ensure the archive parent dir exists before writing summary so a
			// rename failure isn't preceded by an unrelated mkdir failure.
			archiveDir := filepath.Join(cfg.PlansDir, ".archive")
			if err := os.MkdirAll(archiveDir, 0o755); err != nil {
				return err
			}

			// Write summary atomically so a partial summary.md never lands on
			// disk if the process is killed mid-write.
			summary := buildSummary(slug, issues)
			summaryPath := filepath.Join(planDir, "summary.md")
			if err := planrepo.AtomicWrite(summaryPath, []byte(summary), 0o644); err != nil {
				return fmt.Errorf("writing summary: %w", err)
			}

			// Move to archive. If the rename fails, the freshly-written
			// summary.md is now orphaned in planDir — remove it so a retry
			// doesn't see stale summary content from a prior failed attempt.
			dst := filepath.Join(archiveDir, slug)
			if renameErr := os.Rename(planDir, dst); renameErr != nil {
				cleanupErr := os.Remove(summaryPath)
				return errors.Join(fmt.Errorf("moving to archive: %w", renameErr), cleanupErr)
			}

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"status": "ok",
					"file":   dst,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "archived %s to %s\n", slug, dst)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "archive even with active issues")
	return cmd
}

// readIssuesForArchive opens a short-lived planrepo session, copies out the
// issues snapshot, and closes — releasing the plan lock before any of the
// summary write or rename happens. The destructive filesystem move stays
// outside the session: holding the plan lock while moving the directory the
// lock file lives in would tangle release semantics on platforms that resolve
// .pb-lock through the moved path.
func readIssuesForArchive(plansDir, slug string) ([]schema.IssueYaml, error) {
	sess, err := planrepo.NewProd(plansDir).Open(slug)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	snap, err := sess.Snapshot()
	if err != nil {
		return nil, err
	}
	return snap.Issues, nil
}

func buildSummary(slug string, issues []schema.IssueYaml) string {
	var b strings.Builder

	byStatus := make(map[string]int)
	totalPoints := 0
	donePoints := 0
	for _, iss := range issues {
		byStatus[iss.Status]++
		totalPoints += iss.Points
		if iss.Status == "done" {
			donePoints += iss.Points
		}
	}

	b.WriteString(fmt.Sprintf("# Archive: %s\n\n", slug))
	b.WriteString(fmt.Sprintf("Archived: %s\n\n", time.Now().Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("Issues: %d done / %d total\n", byStatus["done"], len(issues)))
	b.WriteString(fmt.Sprintf("Points: %d / %d\n\n", donePoints, totalPoints))
	b.WriteString("## By Status\n\n")

	data, _ := yaml.Marshal(byStatus)
	b.Write(data)

	return b.String()
}
