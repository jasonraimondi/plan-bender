package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewAgentRootCmd creates the root command for the plan-bender-agent binary.
// All output is JSON. Errors are written as {"error": "...", "code": "..."} to stdout.
func NewAgentRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "plan-bender-agent",
		Short:         "plan-bender agent — JSON-only interface for AI agents",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("unknown command; run with --help for usage")
		},
	}

	root.AddCommand(NewAgentValidateCmd())

	return root
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
