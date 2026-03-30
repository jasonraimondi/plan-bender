package main

import (
	"log/slog"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/cli"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := cli.NewAgentRootCmd(version)

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return cli.NewAgentError("failed to get working directory: "+err.Error(), cli.ErrConfigError)
		}

		if _, err := config.Load(wd); err != nil {
			slog.Debug("config load failed", "error", err)
		}
		return nil
	}

	if err := cli.ExecuteAgent(root); err != nil {
		os.Exit(1)
	}
}
