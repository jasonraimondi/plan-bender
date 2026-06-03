package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/spf13/cobra"
)

// Pure read; does not mutate any file.
func NewNextCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "next <slug>",
		Short:   "Show the recommended next issue for a plan",
		Example: "  pb next my-plan",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return NewAgentError("config load failed: "+err.Error(), ErrConfigError)
			}

			repo := planrepo.NewProd(cfg.PlansDir)
			sess, err := repo.Open(slug)
			if err != nil {
				return openErrorToAgent(slug, err)
			}
			defer func() { _ = sess.Close() }()

			result := plan.Resolve(sess.Snapshot().Issues)

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			writeNextHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}
}

// writeNextHuman writes the resolver result as terminal-friendly text.
// One-line title + reason, with skipped tally and any flags worth surfacing.
func writeNextHuman(w io.Writer, r plan.Result) {
	if r.Issue == nil {
		switch {
		case r.AllDone:
			fmt.Fprintln(w, "All issues done — nothing to pick.")
		case r.BlockedCount > 0:
			fmt.Fprintf(w, "No ready issues. %d blocked, %d skipped.\n", r.BlockedCount, len(r.Skipped))
		default:
			fmt.Fprintf(w, "No ready issues. %d skipped.\n", len(r.Skipped))
		}
		return
	}

	flags := ""
	if r.WasBlocked {
		flags += " [stale-blocked]"
	}
	if r.RequiresHuman {
		flags += " [HITL]"
	}
	fmt.Fprintf(w, "#%d %s%s\n", r.Issue.ID, r.Issue.Name, flags)
	fmt.Fprintf(w, "  reason: %s\n", r.Reason)
	if len(r.Skipped) > 0 {
		fmt.Fprintf(w, "  skipped: %d\n", len(r.Skipped))
	}
}
