package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewInitCmd creates the init command.
func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new pb project",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfgPath := filepath.Join(root, ".plan-bender.yaml")

			// Check for existing config
			if _, err := os.Stat(cfgPath); err == nil {
				var overwrite bool
				err := huh.NewConfirm().
					Title(".plan-bender.yaml already exists. Overwrite?").
					Value(&overwrite).
					Run()
				if err != nil {
					return err
				}
				if !overwrite {
					fmt.Fprintln(cmd.OutOrStdout(), "init canceled")
					return nil
				}
			}

			defaults := config.Defaults()

			var backend string
			var plansDir string
			var maxPointsStr string
			var linearAPIKey string
			var linearTeam string

			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Backend").
						Options(
							huh.NewOption("YAML files (local)", "yaml-fs"),
							huh.NewOption("Linear", "linear"),
						).
						Value(&backend),
					huh.NewInput().
						Title("Plans directory").
						Placeholder(defaults.PlansDir).
						Value(&plansDir),
					huh.NewInput().
						Title("Max points per issue").
						Placeholder(strconv.Itoa(defaults.MaxPoints)).
						Value(&maxPointsStr),
				),
				huh.NewGroup(
					huh.NewInput().
						Title("Linear API key").
						Value(&linearAPIKey),
					huh.NewInput().
						Title("Linear team key").
						Value(&linearTeam),
				).WithHideFunc(func() bool {
					return backend != "linear"
				}),
			)

			if err := form.Run(); err != nil {
				return err
			}

			// Build partial config with only non-default values
			partial := make(map[string]any)
			if backend != "" && backend != string(defaults.Backend) {
				partial["backend"] = backend
			}
			if plansDir != "" && plansDir != defaults.PlansDir {
				partial["plans_dir"] = plansDir
			}
			if maxPointsStr != "" {
				mp, err := strconv.Atoi(maxPointsStr)
				if err == nil && mp != defaults.MaxPoints {
					partial["max_points"] = mp
				}
			}
			if backend == "linear" {
				linear := make(map[string]string)
				if linearAPIKey != "" {
					linear["api_key"] = linearAPIKey
				}
				if linearTeam != "" {
					linear["team"] = linearTeam
				}
				if len(linear) > 0 {
					partial["linear"] = linear
				}
			}

			// Write config
			data, err := yaml.Marshal(partial)
			if err != nil {
				return err
			}
			if err := atomicWriteFile(cfgPath, data, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", cfgPath)

			// Generate + install
			installCmd := NewInstallCmd()
			installCmd.SetOut(cmd.OutOrStdout())
			installCmd.SetErr(cmd.ErrOrStderr())
			if err := installCmd.RunE(installCmd, nil); err != nil {
				return fmt.Errorf("install: %w", err)
			}

			// TODO: next steps should direct users to Claude, not raw CLI commands.
			// Users interact via Claude using the installed skills — pb commands are Claude's interface.
			fmt.Fprintln(cmd.OutOrStdout(), "\nNext steps:")
			fmt.Fprintln(cmd.OutOrStdout(), "  Open Claude and ask it to write a PRD or check your plan status.")
			fmt.Fprintln(cmd.OutOrStdout(), "  The installed skills will guide Claude to use pb on your behalf.")

			return nil
		},
	}
}

// removeEmpty removes keys with empty string values from the partial config string
func removeEmpty(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasSuffix(trimmed, ": \"\"") && !strings.HasSuffix(trimmed, ": ''") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
