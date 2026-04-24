package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
)

// NewSyncCmd creates the sync command group. Each backend tool is a subcommand
// (e.g. `sync linear`) which in turn exposes push/pull.
func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync issues with a backend tool",
	}

	cmd.AddCommand(newSyncLinearCmd())
	return cmd
}

func newSyncLinearCmd() *cobra.Command {
	var from string

	cmd := &cobra.Command{
		Use:   "linear <slug>",
		Short: "Sync issues with Linear",
		Long: `Sync local plan with Linear in either direction.

  --from local    push local PRD/issues to Linear (creates project + issues if missing)
  --from linear   pull Linear state into local YAML

Or use the explicit subcommands: 'sync linear push <slug>' / 'sync linear pull <slug>'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch from {
			case "local":
				return syncPush(cmd, args[0])
			case "linear":
				return syncPull(cmd, args[0])
			case "":
				return fmt.Errorf("--from required: 'local' (push) or 'linear' (pull)")
			default:
				return fmt.Errorf("--from must be 'local' or 'linear', got %q", from)
			}
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source of truth: 'local' (push) or 'linear' (pull)")

	cmd.AddCommand(newSyncPushCmd(), newSyncPullCmd())
	return cmd
}

func newSyncPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push <slug>",
		Short: "Push local issues to remote backend",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncPush(cmd, args[0])
		},
	}
}

func newSyncPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <slug>",
		Short: "Pull remote state to local YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncPull(cmd, args[0])
		},
	}
}

func syncPush(cmd *cobra.Command, slug string) error {
	root, _ := os.Getwd()
	ctx := context.Background()

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	be, err := backend.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}

	store := backend.NewProdPlanStore(cfg.PlansDir)
	result, err := backend.SyncPush(ctx, store, be, slug)
	if err != nil {
		return err
	}

	return formatSyncResult(cmd, "push", result)
}

func syncPull(cmd *cobra.Command, slug string) error {
	root, _ := os.Getwd()
	ctx := context.Background()

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	be, err := backend.New(ctx, cfg)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}

	store := backend.NewProdPlanStore(cfg.PlansDir)
	result, err := backend.SyncPull(ctx, store, be, slug)
	if err != nil {
		return err
	}

	return formatSyncResult(cmd, "pull", result)
}

func formatSyncResult(cmd *cobra.Command, op string, result backend.SyncResult) error {
	for _, e := range result.Errors {
		fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", e)
	}

	if isAgentMode(cmd) {
		resp := map[string]any{
			"status":  "ok",
			"created": result.Created,
			"updated": result.Updated,
		}
		if len(result.Errors) > 0 {
			resp["status"] = "partial"
			errs := make([]map[string]any, len(result.Errors))
			for i, e := range result.Errors {
				errs[i] = map[string]any{"issue_id": e.IssueID, "error": e.Err.Error()}
			}
			resp["errors"] = errs
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s complete: %d created, %d updated\n", op, result.Created, result.Updated)
	if len(result.Errors) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%d errors\n", len(result.Errors))
	}
	return nil
}
