package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type contextKey string

const agentJSONOutputKey contextKey = "agentJSONOutput"

// NewAgentRootCmd creates the root command for the plan-bender-agent binary.
// All output is JSON. Errors are written as {"error": "...", "code": "..."} to stdout.
func NewAgentRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "plan-bender-agent",
		Short:         "plan-bender agent — JSON-only interface for AI agents",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := context.WithValue(cmd.Context(), agentJSONOutputKey, true)
			cmd.SetContext(ctx)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("unknown command; run with --help for usage")
		},
	}

	slugComplete := SlugCompletionFunc()

	writePrdCmd := NewWritePrdCmd()
	writePrdCmd.ValidArgsFunction = slugComplete

	writeIssueCmd := NewWriteIssueCmd()
	writeIssueCmd.ValidArgsFunction = slugComplete

	archiveCmd := NewArchiveCmd()
	archiveCmd.ValidArgsFunction = slugComplete

	validateCmd := NewAgentValidateCmd()
	validateCmd.ValidArgsFunction = slugComplete

	syncCmd := NewSyncCmd()
	applySlugCompletionToLeaves(syncCmd, slugComplete)

	root.AddCommand(
		validateCmd,
		NewContextCmd(),
		writePrdCmd,
		writeIssueCmd,
		syncCmd,
		archiveCmd,
	)

	return root
}

// applySlugCompletionToLeaves sets the given completion function on every leaf
// (runnable) command in the subtree rooted at cmd.
func applySlugCompletionToLeaves(cmd *cobra.Command, fn cobra.CompletionFunc) {
	subs := cmd.Commands()
	if len(subs) == 0 {
		cmd.ValidArgsFunction = fn
		return
	}
	for _, sub := range subs {
		applySlugCompletionToLeaves(sub, fn)
	}
}

// isAgentMode returns true when the command is running inside the agent binary.
func isAgentMode(cmd *cobra.Command) bool {
	ctx := cmd.Context()
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(agentJSONOutputKey).(bool)
	return v
}

// ExecuteAgent runs the agent root command and writes JSON errors to stdout on failure.
// Returns the error for the caller to set the exit code.
func ExecuteAgent(root *cobra.Command) error {
	err := root.Execute()
	if err != nil {
		writeErrorJSON(root.OutOrStdout(), err)
	}
	return err
}
