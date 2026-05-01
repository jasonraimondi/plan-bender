package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/spf13/cobra"
)

// NewDispatchCmd creates the `dispatch` command for both binaries.
//
// Exit semantics: ErrHITLOnly is returned unwrapped so main.go can map it to
// exit code 2; other errors propagate and result in exit 1.
func NewDispatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dispatch <slug>",
		Short: "Run the autonomous implementation loop for a plan",
		Args:  cobra.ExactArgs(1),
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
			return d.Run(cmd.Context(), slug)
		},
	}
}

// DispatcherFromConfig builds a Dispatcher with config-driven defaults. Tests
// override .Out and may swap fields after construction.
func DispatcherFromConfig(cfg config.Config, root string) *dispatch.Dispatcher {
	return &dispatch.Dispatcher{Config: cfg, Root: root}
}

// IsHITLOnly reports whether err is the dispatch HITL-only sentinel.
// main.go uses this to map exit code to 2.
func IsHITLOnly(err error) bool {
	return errors.Is(err, dispatch.ErrHITLOnly)
}
