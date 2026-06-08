package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/spf13/cobra"
)

// DispatcherFromConfig builds a Dispatcher with config-driven defaults. Tests
// override .Out and may swap fields after construction.
func DispatcherFromConfig(cfg config.Config, root string) *dispatch.Dispatcher {
	return &dispatch.Dispatcher{Config: cfg, Root: root}
}

// NewMergeCmd integrates a plan's completed (in-review) issues into the
// integration branch in dependency order — the merge-back step lifted out of
// the dispatch loop so it can run on its own. All git work happens inside the
// per-slug integration worktree; the parent repo's HEAD is never touched.
func NewMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "merge <slug>",
		Short: "Merge completed issues into the integration branch in dependency order",
		Long: `Merge a plan's completed work into its integration branch.

Every in-review issue whose blocked_by are all done is merged into the
integration branch (<git-user>/<slug>) in dependency order; merged issues flip
to done and merge conflicts mark the issue blocked (its branch is preserved).
All merges happen inside the per-slug integration worktree — the parent repo's
HEAD is never touched. Running with nothing in-review is a no-op.`,
		Example: "  pb merge my-plan",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, err := config.Load(root)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			d := DispatcherFromConfig(cfg, root)
			d.Out = cmd.OutOrStdout()
			res, err := d.Merge(cmd.Context(), slug)
			if err != nil {
				return err
			}
			return emitMergeOK(cmd, res)
		},
	}
}

func emitMergeOK(cmd *cobra.Command, res dispatch.MergeResult) error {
	if isAgentMode(cmd) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"merged":     intsOrEmpty(res.Merged),
			"conflicted": intsOrEmpty(res.Conflicted),
		})
	}
	out := cmd.OutOrStdout()
	if len(res.Merged) == 0 && len(res.Conflicted) == 0 {
		fmt.Fprintln(out, "nothing to merge: no in-review issues with satisfied dependencies")
		return nil
	}
	if len(res.Merged) > 0 {
		fmt.Fprintf(out, "merged %d issue(s) into the integration branch: %v\n", len(res.Merged), res.Merged)
	}
	if len(res.Conflicted) > 0 {
		fmt.Fprintf(out, "%d issue(s) conflicted and were marked blocked: %v\n", len(res.Conflicted), res.Conflicted)
	}
	return nil
}

// intsOrEmpty maps a nil slice to a non-nil empty one so the JSON encodes as
// [] rather than null — agent consumers expect an array.
func intsOrEmpty(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}
