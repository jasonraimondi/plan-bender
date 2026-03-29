package main

import (
	"log/slog"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/cli"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:     "plan-bender",
		Aliases: []string{"pb"},
		Short:   "Plan-bender CLI — plan management tool",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	root.AddCommand(
		cli.NewInitCmd(),
		cli.NewGenerateSkillsCmd(),
		cli.NewInstallCmd(),
		cli.NewValidateCmd(),
		cli.NewWritePrdCmd(),
		cli.NewWriteIssueCmd(),
		cli.NewStatusCmd(),
		cli.NewGraphCmd(),
		cli.NewSyncCmd(),
		cli.NewArchiveCmd(),
		cli.NewSelfUpdateCmd(version),
	)

	return root
}

