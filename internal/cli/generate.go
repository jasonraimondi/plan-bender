package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	tmpl "github.com/jasonraimondi/plan-bender/internal/template"
	"github.com/spf13/cobra"
)

// NewGenerateSkillsCmd creates the generate-skills command.
func NewGenerateSkillsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate-skills",
		Short: "Generate skill files from templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			templates, err := tmpl.LoadTemplates(root)
			if err != nil {
				return fmt.Errorf("loading templates: %w", err)
			}

			ctx := tmpl.BuildContext(cfg)
			count := 0

			for name, content := range templates {
				skillName := strings.TrimSuffix(name, ".skill.tmpl")
				outDir := filepath.Join(root, ".plan-bender", "skills", skillName)
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return fmt.Errorf("creating dir %s: %w", outDir, err)
				}

				rendered, err := tmpl.Render(name, content, ctx)
				if err != nil {
					return fmt.Errorf("rendering %s: %w", name, err)
				}

				outPath := filepath.Join(outDir, "SKILL.md")
				if err := os.WriteFile(outPath, []byte(rendered), 0o644); err != nil {
					return fmt.Errorf("writing %s: %w", outPath, err)
				}
				count++
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d skills generated\n", count)
			return nil
		},
	}
}
