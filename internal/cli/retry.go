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

// NewRetryCmd creates the `retry` command. Flips a blocked issue back to todo
// under the plan-wide flock by delegating to status.Owner.Transition. The
// from-set is intentionally narrow ([blocked] only); any other current state
// surfaces as a CAS-mismatch error rather than silently overwriting live work.
func NewRetryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retry <slug> <id>",
		Short: "Reset a blocked issue to todo",
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

			owner := dispatch.NewProdStatusOwner(cfg.PlansDir, cfg)
			err = owner.Transition(cmd.Context(), slug, id,
				[]status.Status{status.StatusBlocked}, status.StatusTodo, "retry")

			var casErr *status.ErrCASMismatch
			switch {
			case err == nil:
				return emitRetryOK(cmd, slug, id, "issue #%d: blocked → todo")
			case errors.Is(err, status.ErrAlreadyInState):
				return emitRetryOK(cmd, slug, id, "issue #%d already todo; nothing to do")
			case errors.As(err, &casErr):
				return NewAgentError(
					fmt.Sprintf("issue #%d is %s, not blocked; refusing to retry", id, casErr.Current),
					ErrValidationFailed,
				)
			case strings.Contains(err.Error(), "not found in plan"):
				return NewAgentError(err.Error(), ErrPlanNotFound)
			default:
				return NewAgentError("retry failed: "+err.Error(), ErrInternal)
			}
		},
	}
}

func emitRetryOK(cmd *cobra.Command, slug string, id int, humanFmt string) error {
	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status":     "ok",
			"id":         id,
			"slug":       slug,
			"new_status": "todo",
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), humanFmt+"\n", id)
	return nil
}
