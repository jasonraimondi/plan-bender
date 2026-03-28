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
		initCmd(),
		cli.NewGenerateSkillsCmd(),
		cli.NewInstallCmd(),
		cli.NewValidateCmd(),
		writePRDCmd(),
		writeIssueCmd(),
		statusCmd(),
		graphCmd(),
		syncCmd(),
		archiveCmd(),
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

func initCmd() *cobra.Command {
	return stub("init", "Initialize a new plan-bender project")
}

func writePRDCmd() *cobra.Command {
	return stub("write-prd", "Interactively write a new PRD")
}

func writeIssueCmd() *cobra.Command {
	return stub("write-issue", "Interactively write a new issue")
}

func statusCmd() *cobra.Command {
	return stub("status", "Show plan status dashboard")
}

func graphCmd() *cobra.Command {
	return stub("graph", "Display issue dependency graph")
}

func syncCmd() *cobra.Command {
	return stub("sync", "Sync issues with Linear")
}

func archiveCmd() *cobra.Command {
	return stub("archive", "Archive a completed plan")
}
