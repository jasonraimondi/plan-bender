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

// Re-renders skill templates and refreshes symlinks from the current config
// without touching the config file itself or running Linear setup. Hidden
// from --help: `pb setup` is the documented entry point and runs the same
// code idempotently. `generate` stays callable for scripts that want to skip
// the doctor pass.
func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Regenerate skills from the current config",
		Hidden:  true,
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

			warnStaleTemplateOverrides(root, cmd.ErrOrStderr())

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
// each configured agent and returns the number of skills written.
func GenerateSkills(root string, cfg config.Config, out io.Writer) (int, error) {
	skills, err := tmpl.LoadTemplates(root)
	if err != nil {
		return 0, fmt.Errorf("loading templates: %w", err)
	}

	count := 0
	for _, agent := range cfg.Agents {
		ctx := tmpl.BuildContext(cfg, agent)
		for name, skill := range skills {
			if tmpl.SkillRequiresBackend(name) && !cfg.Linear.Enabled {
				continue
			}
			outDir := filepath.Join(root, ".plan-bender", "skills", agent.Name, name)
			if err := os.RemoveAll(outDir); err != nil {
				return 0, fmt.Errorf("clearing %s: %w", outDir, err)
			}
			if err := writeSkill(name, skill, ctx, outDir); err != nil {
				return 0, err
			}
			count++
		}
	}

	fmt.Fprintf(out, "%d skills generated\n", count)
	return count, nil
}

// writeSkill renders or copies every file in a skill template into outDir. Files
// ending in .tmpl are rendered through the template engine and lose the suffix;
// every other file is copied verbatim. Subdirectories are preserved.
func writeSkill(name string, skill tmpl.Skill, ctx any, outDir string) error {
	for file, content := range skill.Files {
		data := content
		outRel := file
		if strings.HasSuffix(file, ".tmpl") {
			outRel = strings.TrimSuffix(file, ".tmpl")
			rendered, err := tmpl.Render(name+"/"+file, content, ctx)
			if err != nil {
				return fmt.Errorf("rendering %s/%s: %w", name, file, err)
			}
			data = rendered
		}
		outPath := filepath.Join(outDir, filepath.FromSlash(outRel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, []byte(data), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
	}
	return nil
}

// warnStaleTemplateOverrides emits two kinds of stderr warning for stale project
// overrides in .plan-bender/templates/:
//   - a flat {name}.skill.tmpl is no longer read; it must move to
//     {name}/SKILL.md.tmpl, so warn rather than let the fork silently vanish.
//   - a forked copy of a template that upstream now delegates to `pba next`
//     (checked at the new {name}/SKILL.md.tmpl location); re-fork to pick it up.
func warnStaleTemplateOverrides(root string, stderr io.Writer) {
	overrideDir := filepath.Join(root, ".plan-bender", "templates")
	entries, err := os.ReadDir(overrideDir)
	if err != nil {
		return
	}
	watched := map[string]bool{
		"bender-implement-prd": true,
		"bender-orchestrator":  true,
	}
	for _, e := range entries {
		if !e.IsDir() {
			if strings.HasSuffix(e.Name(), ".skill.tmpl") {
				name := strings.TrimSuffix(e.Name(), ".skill.tmpl")
				fmt.Fprintf(stderr, "warning: flat override %s is no longer read; move it to %s/SKILL.md.tmpl\n", e.Name(), name)
			}
			continue
		}
		if watched[e.Name()] {
			if _, err := os.Stat(filepath.Join(overrideDir, e.Name(), "SKILL.md.tmpl")); err == nil {
				fmt.Fprintf(stderr, "warning: forked template %s/SKILL.md.tmpl found; upstream now uses `pba next` — re-fork to pick up the new behavior\n", e.Name())
			}
		}
	}
}
