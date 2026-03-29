package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	tmpl "github.com/jasonraimondi/plan-bender/internal/template"
)

// GenerateSkills renders skill templates into .plan-bender/skills/ and returns
// the number of skills written.
func GenerateSkills(root string, cfg config.Config, out io.Writer) (int, error) {
	templates, err := tmpl.LoadTemplates(root)
	if err != nil {
		return 0, fmt.Errorf("loading templates: %w", err)
	}

	ctx := tmpl.BuildContext(cfg)
	count := 0

	for name, content := range templates {
		skillName := strings.TrimSuffix(name, ".skill.tmpl")
		outDir := filepath.Join(root, ".plan-bender", "skills", skillName)
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

	fmt.Fprintf(out, "%d skills generated\n", count)
	return count, nil
}
