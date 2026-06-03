package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/status"
	"github.com/spf13/cobra"
)

// CompleteMarker formats the completion marker line written to stdout when an
// issue is marked complete. It is a human-readable progress marker, not the
// completion signal: dispatch detects success by re-reading the issue file
// after the subprocess exits (see dispatch.Verdict) and acting on an in-review
// status. The line stays useful in the streamed sub-agent log and for any
// out-of-band tooling that tails it.
func CompleteMarker(id int) string {
	return fmt.Sprintf(`<pba:complete issue-id="%d"/>`, id)
}

// Flips an issue to in-review under the plan-wide flock via
// status.Owner.Transition. The from-set covers todo, in-progress, and backlog
// because sub-agents may skip straight from any of those into in-review on
// completion. Re-completing an already-in-review issue is idempotent: the
// status stays in-review and the OK result (with sentinel) is re-emitted, so a
// retried completion is harmless.
func NewCompleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "complete <slug> <id>",
		Short: "Mark an issue in-review (ready for review)",
		Long: `Flip an issue to in-review and print its completion marker.

The marker line (<pba:complete issue-id="N"/>) is a progress marker for logs
and out-of-band tooling — it is not how dispatch detects success. Dispatch
re-reads the issue after the sub-agent exits and keys on the in-review status.`,
		Example: "  pb complete my-plan 3",
		Args:    exactArgs(2),
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

			owner := planrepo.NewProdStatusOwner(cfg.PlansDir, cfg)
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
	marker := CompleteMarker(id)
	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status": "ok",
			"id":     id,
			"slug":   slug,
			"marker": marker,
		})
	}
	fmt.Fprintln(cmd.OutOrStdout(), marker)
	fmt.Fprintf(cmd.OutOrStdout(), "issue #%d marked in-review\n", id)
	return nil
}
