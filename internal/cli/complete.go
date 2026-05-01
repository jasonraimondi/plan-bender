package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/spf13/cobra"
)

// CompleteSentinel formats the marker line written to stdout when an issue is marked complete.
// Dispatch scans subprocess output for this string to detect successful completion.
func CompleteSentinel(id int) string {
	return fmt.Sprintf(`<pba:complete issue-id="%d"/>`, id)
}

// NewCompleteCmd creates the `complete` command.
func NewCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <slug> <id>",
		Short: "Flip an issue to in-review and emit the dispatch completion sentinel",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return NewAgentError(fmt.Sprintf("invalid issue id %q: must be an integer", args[1]), ErrValidationFailed)
			}

			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return NewAgentError("config load failed: "+err.Error(), ErrConfigError)
			}

			issues, err := plan.LoadIssues(cfg.PlansDir, slug)
			if err != nil {
				return NewAgentError(fmt.Sprintf("plan %q not found: %s", slug, err), ErrPlanNotFound)
			}

			var found *int
			for i := range issues {
				if issues[i].ID == id {
					found = &i
					break
				}
			}
			if found == nil {
				return NewAgentError(fmt.Sprintf("issue #%d not found in plan %q", id, slug), ErrPlanNotFound)
			}

			issue := issues[*found]
			if issue.Status == "done" || issue.Status == "canceled" {
				return NewAgentError(
					fmt.Sprintf("issue #%d is already %s; refusing to overwrite", id, issue.Status),
					ErrValidationFailed,
				)
			}

			issue.Status = "in-review"
			issue.Updated = time.Now().Format("2006-01-02")

			store := backend.NewProdPlanStore(cfg.PlansDir)
			if err := store.WriteIssue(slug, &issue); err != nil {
				return NewAgentError("writing issue: "+err.Error(), ErrInternal)
			}

			sentinel := CompleteSentinel(id)
			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status":   "ok",
					"id":       id,
					"slug":     slug,
					"sentinel": sentinel,
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), sentinel)
			fmt.Fprintf(cmd.OutOrStdout(), "issue #%d marked in-review\n", id)
			return nil
		},
	}
}
