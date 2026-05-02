package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/jasonraimondi/plan-bender/internal/status"
	"github.com/spf13/cobra"
)

// CompleteSentinel formats the marker line written to stdout when an issue is marked complete.
// Dispatch scans subprocess output for this string to detect successful completion.
func CompleteSentinel(id int) string {
	return fmt.Sprintf(`<pba:complete issue-id="%d"/>`, id)
}

// NewCompleteCmd creates the `complete` command. Flips an issue to in-review
// under the plan-wide flock by delegating to status.Owner.Transition. The
// from-set covers todo, in-progress, and backlog because sub-agents may skip
// straight from any of those into in-review on completion. Re-completing an
// already-in-review issue is idempotent: the sentinel is still emitted so a
// dispatcher that lost track of an earlier completion can detect the result.
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

			owner := dispatch.NewProdStatusOwner(cfg.PlansDir)
			err = owner.Transition(cmd.Context(), slug, id,
				[]status.Status{status.StatusTodo, status.StatusInProgress, status.StatusBacklog},
				status.StatusInReview, "")

			var casErr *status.ErrCASMismatch
			switch {
			case err == nil, errors.Is(err, status.ErrAlreadyInState):
				return emitCompleteOK(cmd, slug, id)
			case errors.As(err, &casErr):
				return NewAgentError(
					fmt.Sprintf("issue #%d is already %s; refusing to overwrite", id, casErr.Current),
					ErrValidationFailed,
				)
			case strings.Contains(err.Error(), "not found in plan"):
				return NewAgentError(err.Error(), ErrPlanNotFound)
			default:
				return NewAgentError("complete failed: "+err.Error(), ErrInternal)
			}
		},
	}
}

func emitCompleteOK(cmd *cobra.Command, slug string, id int) error {
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
}
