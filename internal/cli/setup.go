package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/jasonraimondi/plan-bender/internal/agents"
	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/spf13/cobra"
)

// linearValidator validates Linear credentials.
type linearValidator interface {
	ListWorkflowStates(ctx context.Context, teamID string) (string, map[string]string, string, error)
}

type setupDeps struct {
	version      string
	newValidator func(apiKey string) linearValidator
}

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
	cfgPath := filepath.Join(root, ".plan-bender.json")
	localPath := filepath.Join(root, ".plan-bender.local.json")

	// Skip creation when .plan-bender.local.json already exists — the user is
	// intentionally using only the local layer, and the config loader handles
	// the merge correctly without a project-level file.
	created := false
	localOnly := false
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if _, localErr := os.Stat(localPath); localErr == nil {
			localOnly = true
		} else {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "  ")
			if err := enc.Encode(config.StarterConfig()); err != nil {
				return err
			}
			if err := backend.AtomicWrite(cfgPath, buf.Bytes(), configFileMode(cfgPath)); err != nil {
				return err
			}
			created = true
		}
	}

	// Backfill the $schema reference into pre-existing config files so editors
	// pick up validation/autocomplete. A freshly-created file already has it.
	var schemaAdded []string
	if !created {
		for _, p := range []string{cfgPath, localPath} {
			if _, err := os.Stat(p); err != nil {
				continue
			}
			added, err := ensureSchemaField(p)
			if err != nil {
				return err
			}
			if added {
				schemaAdded = append(schemaAdded, filepath.Base(p))
			}
		}
	}

	if useLinear {
		if err := setupLinear(root, deps, yes); err != nil {
			return err
		}
	}

	cfg, err := config.Load(root)
	if err != nil {
		var cfgErr *config.ConfigError
		if errors.As(err, &cfgErr) {
			fmt.Fprint(cmd.ErrOrStderr(), cfgErr.FormatHuman())
			return fmt.Errorf("config validation failed")
		}
		return err
	}

	if _, err := GenerateSkills(root, cfg, out); err != nil {
		return err
	}

	warnStaleTemplateOverrides(root, cmd.ErrOrStderr())

	count, err := symlinkSkills(root, cfg)
	if err != nil {
		return err
	}

	if cfg.ManageGitignore {
		if err := ensureGitignoreForAgents(root, cfg.Agents); err != nil {
			return fmt.Errorf("updating .gitignore: %w", err)
		}
	}

	switch {
	case created:
		fmt.Fprintf(out, "Config:  .plan-bender.json (created)\n")
	case localOnly:
		fmt.Fprintf(out, "Config:  .plan-bender.local.json (local only — no project file written)\n")
		fmt.Fprintf(out, "         If teammates clone this repo they'll get the starter config; your\n")
		fmt.Fprintf(out, "         local overrides stay in .plan-bender.local.json.\n")
	default:
		fmt.Fprintf(out, "Config:  .plan-bender.json (exists)\n")
	}

	if len(schemaAdded) > 0 {
		fmt.Fprintf(out, "Schema:  added $schema to %s\n", strings.Join(schemaAdded, ", "))
	}

	if cfg.Linear.Enabled {
		fmt.Fprintf(out, "Linear:  enabled — team %s\n", cfg.Linear.Team)
	} else {
		fmt.Fprintf(out, "Linear:  disabled\n")
	}

	fmt.Fprintf(out, "Skills:  %d installed\n", count)
	fmt.Fprintf(out, "Plans:   %s\n", cfg.PlansDir)

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
	fmt.Fprintf(out, "  In your agent (Claude Code, Pi, etc.):\n")
	fmt.Fprintf(out, "    /bender-orchestrator    — see your planning dashboard\n")
	fmt.Fprintf(out, "    /bender-write-prd       — start a new plan\n")
	fmt.Fprintf(out, "  From the shell:\n")
	fmt.Fprintf(out, "    pb status <slug>           — per-issue state for a plan\n")
	fmt.Fprintf(out, "    pb dispatch <slug>         — autonomous implementation loop\n")
	if useLinear {
		fmt.Fprintf(out, "    pb sync linear push <slug> — push existing plan to Linear (creates project + issues)\n")
	}

	return nil
}

func setupLinear(root string, deps setupDeps, yes bool) error {
	cfgPath := filepath.Join(root, ".plan-bender.json")
	localPath := filepath.Join(root, ".plan-bender.local.json")

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	validator := deps.newValidator(apiKey)
	if _, _, _, err := validator.ListWorkflowStates(ctx, team); err != nil {
		return fmt.Errorf("linear credential validation failed: %w", err)
	}

	if err := mergeJSONFile(cfgPath, map[string]any{
		"linear": map[string]any{"enabled": true},
	}); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	if err := mergeJSONFile(localPath, map[string]any{
		"linear": map[string]any{"api_key": apiKey, "team": team},
	}); err != nil {
		return fmt.Errorf("updating local config: %w", err)
	}

	return nil
}

// configFileMode picks the umask for a written config file. `.plan-bender.local.json`
// can hold Linear API keys, so it is restricted to owner-only (0o600); the
// project and global tiers stay readable (0o644). Centralized so every writer
// (setup, migrate, future tooling) cannot accidentally widen permissions.
func configFileMode(path string) os.FileMode {
	if filepath.Base(path) == ".plan-bender.local.json" {
		return 0o600
	}
	return 0o644
}

// ensureSchemaField backfills the "$schema" key into an existing config file
// when missing, inserting it as the first key while preserving the rest of the
// file's formatting and any unknown keys. Returns true when it wrote a change.
func ensureSchemaField(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		// Not an object we can edit safely — leave it for the user to fix.
		return false, nil
	}
	if _, ok := obj["$schema"]; ok {
		return false, nil
	}

	field := fmt.Sprintf("%q: %q", "$schema", config.SchemaURL)
	var out []byte
	if len(obj) == 0 {
		out = []byte("{\n  " + field + "\n}\n")
	} else {
		brace := bytes.IndexByte(data, '{')
		if brace < 0 {
			return false, nil
		}
		out = append(out, data[:brace+1]...)
		out = append(out, "\n  "+field+","...)
		out = append(out, data[brace+1:]...)
	}

	if err := backend.AtomicWrite(path, out, configFileMode(path)); err != nil {
		return false, err
	}
	return true, nil
}

// mergeJSONFile reads an existing JSON file (or starts empty), deep-merges the updates, and writes back.
func mergeJSONFile(path string, updates map[string]any) error {
	raw := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parsing existing %s: %w", path, err)
		}
		if raw == nil {
			raw = make(map[string]any)
		}
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

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(raw); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return backend.AtomicWrite(path, buf.Bytes(), configFileMode(path))
}

// symlinkSkills creates symlinks from generated skill dirs into each configured agent's target directory.
func symlinkSkills(root string, cfg config.Config) (int, error) {
	count := 0
	for _, agent := range cfg.Agents {
		sourceDir := filepath.Join(root, ".plan-bender", "skills", agent.Name)
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return 0, fmt.Errorf("reading skills dir for agent %s: %w", agent.Name, err)
		}

		targetDir, err := resolveAgentDir(root, agent)
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
				return 0, fmt.Errorf("symlinking %s to %s: %w", e.Name(), agent.Name, err)
			}
			count++
		}
	}

	return count, nil
}

// resolveAgentDir returns the absolute target directory for an agent based on its scope.
func resolveAgentDir(root string, agent config.ResolvedAgent) (string, error) {
	switch agent.Scope {
	case agents.UserOnly:
		return expandHome(agent.UserDir)
	default:
		return filepath.Join(root, agent.ProjectDir), nil
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
func ensureGitignoreForAgents(root string, agts []config.ResolvedAgent) error {
	entries := []string{".plan-bender/", ".plan-bender.local.json"}

	for _, agent := range agts {
		if agent.Scope == agents.UserOnly {
			continue
		}
		if agent.GitignorePattern != "" {
			entries = append(entries, agent.GitignorePattern)
		}
	}

	gitignorePath := filepath.Join(root, ".gitignore")
	existing, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", gitignorePath, err)
	}
	content := string(existing)

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(toAdd, "\n") + "\n"
	if err := os.WriteFile(gitignorePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", gitignorePath, err)
	}
	return nil
}
