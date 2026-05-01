package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/cli"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/update"
	"github.com/spf13/cobra"
)

var version = "dev"

// checkForUpdateFn is the function used to check for updates.
// Package-level variable for testability.
var checkForUpdateFn = func(currentVersion string) (string, bool, error) {
	return update.CheckForUpdate(currentVersion, nil, false)
}

type updateResult struct {
	latest  string
	isNewer bool
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool
	var updateCh chan updateResult

	root := &cobra.Command{
		Use:     "plan-bender",
		Aliases: []string{"pb"},
		Short:   "pb — plan management tool",
		Long:    "Plan-bender CLI for humans.\n\nFor agent commands (context, validate, write), use plan-bender-agent (pba).",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

			if !shouldCheckForUpdate(cmd) {
				return
			}

			updateCh = make(chan updateResult, 1)
			go func() {
				latest, isNewer, err := checkForUpdateFn(version)
				if err != nil {
					slog.Debug("update check failed", "error", err)
					updateCh <- updateResult{}
					return
				}
				updateCh <- updateResult{latest: latest, isNewer: isNewer}
			}()
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if updateCh == nil {
				return
			}

			select {
			case result := <-updateCh:
				if result.isNewer {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"\nA new version of pb is available: v%s → v%s\n  Run 'pb self-update' to upgrade\n",
						version, result.latest)
				}
			case <-time.After(500 * time.Millisecond):
				slog.Debug("update check timed out")
			}
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	root.AddCommand(
		cli.NewSetupCmd(version),
		cli.NewGenerateCmd(),
		cli.NewSyncCmd(),
		cli.NewDoctorCmd(version),
		cli.NewSelfUpdateCmd(version),
		cli.NewCompletionCmd(),
		cli.NewDocsCmd(),
		cli.NewNextCmd(),
		cli.NewCompleteCmd(),
		cli.NewWorktreeCmd(),
	)

	return root
}

func shouldCheckForUpdate(cmd *cobra.Command) bool {
	if version == "dev" {
		return false
	}

	if os.Getenv("PB_NO_UPDATE_CHECK") == "1" {
		return false
	}

	if cmd.Name() == "self-update" || cmd.CalledAs() == "self-update" {
		return false
	}

	wd, err := os.Getwd()
	if err != nil {
		slog.Debug("could not get working directory for config", "error", err)
		return true
	}

	cfg, err := config.Load(wd)
	if err != nil {
		slog.Debug("could not load config for update check", "error", err)
		return true
	}

	return cfg.UpdateCheck
}
