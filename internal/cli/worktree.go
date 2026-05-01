package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/worktree"
	"github.com/spf13/cobra"
)

// NewWorktreeCmd returns the `worktree` parent command with `create` and `gc` subcommands.
func NewWorktreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage plan-bender git worktrees",
	}
	cmd.AddCommand(newWorktreeCreateCmd(), newWorktreeGCCmd())
	return cmd
}

func newWorktreeCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <slug> <issue-id>",
		Short: "Create a worktree for one issue",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			id, err := strconv.Atoi(args[1])
			if err != nil {
				return NewAgentError(fmt.Sprintf("invalid issue id %q", args[1]), ErrValidationFailed)
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

			res, err := worktree.Create(root, slug, id, issue.Slug, "")
			if err != nil {
				return NewAgentError("creating worktree: "+err.Error(), ErrInternal)
			}

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"path":   res.Path,
					"branch": res.Branch,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "worktree: %s\nbranch:   %s\n", res.Path, res.Branch)
			return nil
		},
	}
}

func newWorktreeGCCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gc <slug>",
		Short: "Remove worktrees and branches for a plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, _ := os.Getwd()

			if _, err := config.Load(root); err != nil {
				return NewAgentError("config load failed: "+err.Error(), ErrConfigError)
			}

			removed, err := worktree.GC(root, slug)
			if err != nil {
				return NewAgentError("gc failed: "+err.Error(), ErrInternal)
			}
			if removed == nil {
				removed = []string{}
			}

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"removed": removed,
				})
			}
			if len(removed) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no worktrees to remove")
				return nil
			}
			for _, p := range removed {
				fmt.Fprintf(cmd.OutOrStdout(), "removed: %s\n", p)
			}
			return nil
		},
	}
}
