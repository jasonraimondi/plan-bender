package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/jasonraimondi/plan-bender/internal/agents"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewSetupCmd creates the setup command.
// Idempotent: if config exists, skips the interactive form and just regenerates + re-symlinks skills.
func NewSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up or refresh a pb project",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfgPath := filepath.Join(root, ".plan-bender.yaml")

			var cfg config.Config

			if _, err := os.Stat(cfgPath); err != nil {
				// No config — run interactive form
				partial, err := runSetupForm()
				if err != nil {
					return err
				}

				data, err := yaml.Marshal(partial)
				if err != nil {
					return err
				}
				if err := atomicWriteFile(cfgPath, data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", cfgPath)
			}

			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			if _, err := GenerateSkills(root, cfg, cmd.OutOrStdout()); err != nil {
				return err
			}

			count, err := symlinkSkills(root, cfg)
			if err != nil {
				return err
			}

			ensureGitignoreForAgents(root, cfg.Agents)

			fmt.Fprintf(cmd.OutOrStdout(), "%d skills installed\n", count)

			fmt.Fprintln(cmd.OutOrStdout(), "\nNext steps:")
			fmt.Fprintln(cmd.OutOrStdout(), "  Open Claude and ask it to write a PRD or check your plan status.")
			fmt.Fprintln(cmd.OutOrStdout(), "  The installed skills will guide Claude to use pb on your behalf.")

			return nil
		},
	}
}

func runSetupForm() (map[string]any, error) {
	defaults := config.Defaults()

	var backend string
	var plansDir string
	var maxPointsStr string
	var selectedAgents []string
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
			huh.NewMultiSelect[string]().
				Title("Agents").
				Options(agentOptions()...).
				Value(&selectedAgents),
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
		return nil, err
	}

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
	if len(selectedAgents) > 0 {
		partial["agents"] = selectedAgents
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

	return partial, nil
}

func agentOptions() []huh.Option[string] {
	names := agents.Names()
	opts := make([]huh.Option[string], len(names))
	for i, name := range names {
		opt := huh.NewOption(name, name)
		if name == "claude-code" {
			opt = opt.Selected(true)
		}
		opts[i] = opt
	}
	return opts
}

// symlinkSkills creates symlinks from generated skill dirs into each configured agent's target directory.
func symlinkSkills(root string, cfg config.Config) (int, error) {
	sourceDir := filepath.Join(root, ".plan-bender", "skills")
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return 0, fmt.Errorf("reading skills dir: %w", err)
	}

	count := 0
	for _, agentName := range cfg.Agents {
		ac, ok := agents.Get(agentName)
		if !ok {
			return 0, fmt.Errorf("unknown agent %q", agentName)
		}

		targetDir, err := resolveAgentDir(root, ac)
		if err != nil {
			return 0, err
		}

		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return 0, fmt.Errorf("creating target dir: %w", err)
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			src := filepath.Join(sourceDir, e.Name())
			dst := filepath.Join(targetDir, e.Name())

			info, err := os.Lstat(dst)
			if err == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					os.Remove(dst)
				} else {
					continue
				}
			}

			if err := os.Symlink(src, dst); err != nil {
				return 0, fmt.Errorf("symlinking %s to %s: %w", e.Name(), agentName, err)
			}
			count++
		}
	}

	return count, nil
}

// resolveAgentDir returns the absolute target directory for an agent based on its scope.
func resolveAgentDir(root string, ac agents.AgentConfig) (string, error) {
	switch ac.Scope {
	case agents.UserOnly:
		return expandHome(ac.UserDir)
	default:
		return filepath.Join(root, ac.ProjectDir), nil
	}
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}

// ensureGitignoreForAgents writes registry-driven gitignore patterns for project-scoped agents.
func ensureGitignoreForAgents(root string, agentNames []string) {
	entries := []string{".plan-bender/", ".plan-bender.local.yaml"}

	for _, name := range agentNames {
		ac, ok := agents.Get(name)
		if !ok {
			continue
		}
		if ac.Scope == agents.UserOnly {
			continue
		}
		if ac.GitignorePattern != "" {
			entries = append(entries, ac.GitignorePattern)
		}
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	existing, _ := os.ReadFile(gitignorePath)
	content := string(existing)

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return
	}

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(toAdd, "\n") + "\n"
	os.WriteFile(gitignorePath, []byte(content), 0o644)
}
