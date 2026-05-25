package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/dispatch"
	"github.com/spf13/cobra"
)

// Exit semantics: ErrHITLOnly is returned unwrapped so main.go can map it to
// exit code 2; other errors propagate and result in exit 1.
func NewDispatchCmd(version string) *cobra.Command {
	var base string
	cmd := &cobra.Command{
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

			if base != "" {
				if err := validateBaseRef(cmd.Context(), root, base); err != nil {
					return err
				}
			}

			d := DispatcherFromConfig(cfg, root)
			d.Out = cmd.OutOrStdout()
			d.Base = base
			err = d.Run(cmd.Context(), slug)

			// A setup failure happens here in Go before any sub-agent runs, so
			// the agent-facing report_bugs prompt never fires — write the artifact
			// from the failing command itself. Limited to the setup-failure class:
			// stuck-on-blocked and lock contention are user-resolvable, not bugs,
			// and HITL-only is a clean exit.
			if IsSetupFailure(err) && cfg.ReportBugs {
				if path, werr := writeBugReport(root, version, "dispatch "+slug, err); werr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to write bug report: %v\n", werr)
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "wrote bug report to %s — please file at https://github.com/jasonraimondi/plan-bender/issues\n", path)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "fork the integration branch off this commit-ish (default: repo default branch)")
	return cmd
}

// validateBaseRef rejects refs that don't resolve to a commit. The `^{commit}`
// suffix dereferences annotated tags etc., so blobs and trees error here
// instead of producing a useless `git branch` failure deep in the dispatch loop.
func validateBaseRef(ctx context.Context, root, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("--base %q does not resolve to a commit in %s", ref, root)
	}
	return nil
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

// IsSetupFailure reports whether err is the dispatch all-setup-failed sentinel:
// every ready issue failed environment setup before any sub-agent ran. This is
// the only exit-1 shape the report_bugs artifact covers, since stuck-on-blocked
// and lock contention are user-resolvable rather than plan-bender bugs.
func IsSetupFailure(err error) bool {
	return errors.Is(err, dispatch.ErrSetupFailed)
}
