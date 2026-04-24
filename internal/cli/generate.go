package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	tmpl "github.com/jasonraimondi/plan-bender/internal/template"
	"github.com/spf13/cobra"
)

// NewGenerateCmd creates the generate command, which re-renders skill
// templates and refreshes symlinks from the current config without touching
// the config file itself or running Linear setup.
func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Regenerate skills from the current config",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
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

			out := cmd.OutOrStdout()
			if _, err := GenerateSkills(root, cfg, out); err != nil {
				return err
			}

			count, err := symlinkSkills(root, cfg)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "Skills:  %d installed\n", count)
			return nil
		},
	}
	return cmd
}

// GenerateSkills renders skill templates into .plan-bender/skills/{agent}/ for
// each configured agent and returns the number of skill files written.
func GenerateSkills(root string, cfg config.Config, out io.Writer) (int, error) {
	templates, err := tmpl.LoadTemplates(root)
	if err != nil {
		return 0, fmt.Errorf("loading templates: %w", err)
	}

	count := 0

	for _, agent := range cfg.Agents {
		ctx := tmpl.BuildContext(cfg, agent)

		for name, content := range templates {
			skillName := strings.TrimSuffix(name, ".skill.tmpl")
			if tmpl.SkillRequiresBackend(skillName) && !cfg.Linear.Enabled {
				continue
			}
			outDir := filepath.Join(root, ".plan-bender", "skills", agent.Name, skillName)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return 0, fmt.Errorf("creating dir %s: %w", outDir, err)
			}

			rendered, err := tmpl.Render(name, content, ctx)
			if err != nil {
				return 0, fmt.Errorf("rendering %s: %w", name, err)
			}

			outPath := filepath.Join(outDir, "SKILL.md")
			if err := os.WriteFile(outPath, []byte(rendered), 0o644); err != nil {
				return 0, fmt.Errorf("writing %s: %w", outPath, err)
			}
			count++
		}
	}

	fmt.Fprintf(out, "%d skills generated\n", count)
	return count, nil
}
