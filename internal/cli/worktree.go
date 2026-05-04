package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/status"
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

			// Load issue slug through a planrepo session so this command shares
			// one persistence boundary with status writes. Close the session before
			// worktree.Create + Claim so we don't hold the plan lock across the
			// `git worktree add` subprocess (and so Claim's session can reacquire).
			issueSlug, err := lookupIssueSlug(cfg.PlansDir, slug, id)
			if err != nil {
				return err
			}

			res, err := worktree.Create(cmd.Context(), root, slug, id, issueSlug, "")
			if err != nil {
				return NewAgentError("creating worktree: "+err.Error(), ErrInternal)
			}

			// Atomic claim: flip status to in-progress and stamp the branch field
			// so the YAML reflects the on-disk worktree. Without this the branch
			// lives on disk while the issue YAML still shows backlog/null branch,
			// and dispatch's CAS-checks lose the thread on the next iteration.
			owner := dispatch.NewProdStatusOwner(cfg.PlansDir, cfg)
			claimErr := owner.Claim(cmd.Context(), slug, id, res.Branch, "worktree create")
			claimed := claimErr == nil || errors.Is(claimErr, status.ErrAlreadyInState)
			if !claimed {
				// The lookup→Create→Claim flow releases the plan lock between
				// lookup and Claim. If a concurrent writer changed the issue's
				// state in that window, the worktree we just created is now
				// orphaned. When res.Created is true we materialized this pair
				// in this call, so it's safe to roll back the artifacts; when
				// false, leave them alone (they predate this call).
				rollbackNote := ""
				if res.Created {
					if rmErr := worktree.Remove(cmd.Context(), root, res.Path, res.Branch); rmErr != nil {
						rollbackNote = fmt.Sprintf(" (rollback also failed: %v; run `pba worktree gc %s` manually)", rmErr, slug)
					} else {
						rollbackNote = " (worktree and branch rolled back)"
					}
				}
				var casErr *status.ErrCASMismatch
				if errors.As(claimErr, &casErr) {
					return NewAgentError(
						fmt.Sprintf("worktree created at %s on branch %s, but issue #%d is %s — refusing to claim a non-actionable issue%s",
							res.Path, res.Branch, id, casErr.Current, rollbackNote),
						ErrValidationFailed,
					)
				}
				return NewAgentError(
					fmt.Sprintf("worktree created at %s on branch %s, but failed to update issue #%d YAML: %v%s",
						res.Path, res.Branch, id, claimErr, rollbackNote),
					ErrInternal,
				)
			}

			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"path":   res.Path,
					"branch": res.Branch,
					"status": string(status.StatusInProgress),
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "worktree: %s\nbranch:   %s\nstatus:   %s\n", res.Path, res.Branch, status.StatusInProgress)
			return nil
		},
	}
}

// lookupIssueSlug opens a short-lived planrepo session to read one issue's
// slug, then releases the lock. The lock is intentionally released before
// `worktree create` and `Claim` run so the subsequent Claim session can
// reacquire without deadlocking against the read.
func lookupIssueSlug(plansDir, slug string, id int) (string, error) {
	plans := planrepo.NewProd(plansDir)
	sess, err := plans.Open(slug)
	if err != nil {
		return "", NewAgentError(fmt.Sprintf("plan %q not found: %s", slug, err), ErrPlanNotFound)
	}
	defer sess.Close()
	snap, err := sess.Snapshot()
	if err != nil {
		return "", NewAgentError("reading snapshot: "+err.Error(), ErrInternal)
	}
	for _, iss := range snap.Issues {
		if iss.ID == id {
			return iss.Slug, nil
		}
	}
	return "", NewAgentError(fmt.Sprintf("issue #%d not found in plan %q", id, slug), ErrPlanNotFound)
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

			removed, err := worktree.GC(cmd.Context(), root, slug, nil, cmd.ErrOrStderr())
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
