package main

import (
	"fmt"
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
		syncCmd(),
		cli.NewArchiveCmd(),
	)

	return root
}

func stub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: not implemented\n", name)
			return nil
		},
	}
}

func syncCmd() *cobra.Command {
	return stub("sync", "Sync issues with Linear")
}
