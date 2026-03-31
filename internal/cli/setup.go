package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/jasonraimondi/plan-bender/internal/agents"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// linearValidator validates Linear credentials.
type linearValidator interface {
	ListWorkflowStates(ctx context.Context, teamID string) (map[string]string, error)
}

type setupDeps struct {
	version      string
	newValidator func(apiKey string) linearValidator
}

// NewSetupCmd creates the setup command.
func NewSetupCmd(version string) *cobra.Command {
	return newSetupCmd(setupDeps{version: version})
}

func newSetupCmd(deps setupDeps) *cobra.Command {
	var yes, useLinear bool

	if deps.newValidator == nil {
		deps.newValidator = func(apiKey string) linearValidator {
			return linear.NewClient(apiKey)
		}
	}

	cmd := &cobra.Command{
		Use:     "setup",
		Aliases: []string{"init"},
		Short:   "Set up or refresh a pb project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(cmd, deps, yes, useLinear)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Non-interactive mode")
	cmd.Flags().BoolVar(&useLinear, "linear", false, "Configure Linear integration")
	return cmd
}

func runSetup(cmd *cobra.Command, deps setupDeps, yes, useLinear bool) error {
	root, _ := os.Getwd()
	out := cmd.OutOrStdout()
	cfgPath := filepath.Join(root, ".plan-bender.yaml")

	// 1. Write defaults if no config exists
	created := false
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		defaults := config.Defaults()
		data, err := yaml.Marshal(defaults)
		if err != nil {
			return err
		}
		if err := atomicWriteFile(cfgPath, data, 0o644); err != nil {
			return err
		}
		created = true
	}

	// 2. Handle --linear
	if useLinear {
		if err := setupLinear(root, deps, yes); err != nil {
			return err
		}
	}

	// 3. Load merged config
	cfg, err := config.Load(root)
	if err != nil {
		var cfgErr *config.ConfigError
		if errors.As(err, &cfgErr) {
			fmt.Fprint(cmd.ErrOrStderr(), cfgErr.FormatHuman())
			return fmt.Errorf("config validation failed")
		}
		return err
	}

	// 4. Generate + symlink skills
	if _, err := GenerateSkills(root, cfg, out); err != nil {
		return err
	}

	count, err := symlinkSkills(root, cfg)
	if err != nil {
		return err
	}

	ensureGitignoreForAgents(root, cfg.Agents)

	// 5. Output summary
	if created {
		fmt.Fprintf(out, "Config:  .plan-bender.yaml (created)\n")
	} else {
		fmt.Fprintf(out, "Config:  .plan-bender.yaml (exists)\n")
	}

	if cfg.Linear.Enabled {
		fmt.Fprintf(out, "Linear:  enabled — team %s\n", cfg.Linear.Team)
	} else {
		fmt.Fprintf(out, "Linear:  disabled\n")
	}

	fmt.Fprintf(out, "Skills:  %d installed\n", count)
	fmt.Fprintf(out, "Plans:   %s\n", cfg.PlansDir)

	// 6. Inline doctor checks (warnings only, don't block)
	fmt.Fprintf(out, "\nHealth:\n")
	results := RunChecks(root, cfg, deps.version)
	for _, r := range results {
		if r.Pass {
			line := fmt.Sprintf("  \u2713 %s", r.Name)
			if r.Message != "" {
				line += " \u2014 " + r.Message
			}
			fmt.Fprintln(out, line)
		} else {
			fmt.Fprintf(out, "  \u2717 %s \u2014 %s\n", r.Name, r.Message)
		}
	}

	fmt.Fprintf(out, "\nReady! Next:\n")
	fmt.Fprintf(out, "  /bender-orchestrator    — see your planning dashboard\n")
	fmt.Fprintf(out, "  /bender-write-prd       — start a new plan\n")

	return nil
}

func setupLinear(root string, deps setupDeps, yes bool) error {
	cfgPath := filepath.Join(root, ".plan-bender.yaml")
	localPath := filepath.Join(root, ".plan-bender.local.yaml")

	// Get credentials from env vars or prompts
	apiKey := os.Getenv("LINEAR_API_KEY")
	team := os.Getenv("LINEAR_TEAM")

	if apiKey == "" || team == "" {
		if yes {
			return fmt.Errorf("--linear requires $LINEAR_API_KEY and $LINEAR_TEAM in non-interactive mode")
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Linear API key").
					EchoMode(huh.EchoModePassword).
					Value(&apiKey),
				huh.NewInput().
					Title("Linear team key").
					Value(&team),
			),
		)
		if err := form.Run(); err != nil {
			return fmt.Errorf("linear setup: %w", err)
		}
	}

	if apiKey == "" || team == "" {
		return fmt.Errorf("linear API key and team are required")
	}

	// Validate credentials
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	validator := deps.newValidator(apiKey)
	if _, err := validator.ListWorkflowStates(ctx, team); err != nil {
		return fmt.Errorf("linear credential validation failed: %w", err)
	}

	// Write linear.enabled: true to project config
	if err := mergeYAMLFile(cfgPath, map[string]any{
		"linear": map[string]any{"enabled": true},
	}); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	// Write credentials to local config (gitignored)
	if err := mergeYAMLFile(localPath, map[string]any{
		"linear": map[string]any{"api_key": apiKey, "team": team},
	}); err != nil {
		return fmt.Errorf("updating local config: %w", err)
	}

	return nil
}

// mergeYAMLFile reads an existing YAML file (or starts empty), deep-merges the updates, and writes back.
func mergeYAMLFile(path string, updates map[string]any) error {
	raw := make(map[string]any)
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &raw)
	}

	for k, v := range updates {
		existing, _ := raw[k].(map[string]any)
		incoming, ok := v.(map[string]any)
		if ok && existing != nil {
			for ik, iv := range incoming {
				existing[ik] = iv
			}
			raw[k] = existing
		} else {
			raw[k] = v
		}
	}

	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWriteFile(path, out, 0o644)
}

// symlinkSkills creates symlinks from generated skill dirs into each configured agent's target directory.
func symlinkSkills(root string, cfg config.Config) (int, error) {
	count := 0
	for _, agentName := range cfg.Agents {
		ac, err := agents.Get(agentName)
		if err != nil {
			return 0, fmt.Errorf("unknown agent %q", agentName)
		}

		sourceDir := filepath.Join(root, ".plan-bender", "skills", agentName)
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return 0, fmt.Errorf("reading skills dir for agent %s: %w", agentName, err)
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
		ac, err := agents.Get(name)
		if err != nil {
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
