package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// exactArgs wraps cobra.ExactArgs but appends the command's usage line on
// failure. The roots set SilenceUsage, so a bare "accepts N arg(s)" otherwise
// leaves the user without the positional-arg names; the usage line restores
// them (e.g. "usage: plan-bender complete <slug> <id>").
func exactArgs(n int) cobra.PositionalArgs {
	base := cobra.ExactArgs(n)
	return func(cmd *cobra.Command, args []string) error {
		if err := base(cmd, args); err != nil {
			return fmt.Errorf("%w\nusage: %s", err, cmd.UseLine())
		}
		return nil
	}
}
