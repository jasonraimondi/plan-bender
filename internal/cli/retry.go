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

// NewRetryCmd creates the `retry` command. Flips a blocked issue back to todo
// and clears its notes so the dispatcher will re-pick it on the next run.
// Refuses any non-blocked status to avoid clobbering live state.
func NewRetryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry <slug> <id>",
		Short: "Reset a blocked issue to todo and clear its failure notes",
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
			if issue.Status != "blocked" {
				return NewAgentError(
					fmt.Sprintf("issue #%d is %s, not blocked; refusing to retry", id, issue.Status),
					ErrValidationFailed,
				)
			}

			previousNotes := ""
			if issue.Notes != nil {
				previousNotes = *issue.Notes
			}

			issue.Status = "todo"
			issue.Notes = nil
			issue.Updated = time.Now().Format("2006-01-02")

			store := backend.NewProdPlanStore(cfg.PlansDir)
			if err := store.WriteIssue(slug, &issue); err != nil {
				return NewAgentError("writing issue: "+err.Error(), ErrInternal)
			}

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"status":         "ok",
					"id":             id,
					"slug":           slug,
					"new_status":     "todo",
					"cleared_notes":  previousNotes,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "issue #%d: blocked → todo\n", id)
			if previousNotes != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "cleared notes")
			}
			return nil
		},
	}
}
