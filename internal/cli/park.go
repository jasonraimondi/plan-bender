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

// Flips an in-progress issue to needs-input under the plan-wide flock via
// status.Owner.Transition. The from-set is intentionally narrow ([in-progress]
// only); any other current state surfaces as a CAS-mismatch error rather
// than silently overwriting live work.
func NewParkCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "park <slug> <id>",
		Short:   "Park an in-progress issue as needs-input",
		Example: "  pb park my-plan 3",
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
				[]status.Status{status.StatusInProgress}, status.StatusNeedsInput, "park")

			var casErr *status.ErrCASMismatch
			switch {
			case err == nil:
				return emitParkOK(cmd, slug, id, "issue #%d: in-progress → needs-input")
			case errors.Is(err, status.ErrAlreadyInState):
				return emitParkOK(cmd, slug, id, "issue #%d already needs-input; nothing to do")
			case errors.As(err, &casErr):
				return NewAgentError(
					fmt.Sprintf("issue #%d is %s, not in-progress; refusing to park", id, casErr.Current),
					ErrValidationFailed,
				)
			case strings.Contains(err.Error(), "not found in plan"):
				return NewAgentError(err.Error(), ErrPlanNotFound)
			default:
				return NewAgentError("park failed: "+err.Error(), ErrInternal)
			}
		},
	}
}

func emitParkOK(cmd *cobra.Command, slug string, id int, humanFmt string) error {
	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"status":     "ok",
			"id":         id,
			"slug":       slug,
			"new_status": "needs-input",
		})
	}
	fmt.Fprintf(cmd.OutOrStdout(), humanFmt+"\n", id)
	return nil
}
