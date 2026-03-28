package template

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:embedded/*.tmpl
var embeddedFS embed.FS

// LoadTemplates returns a map of template name -> content.
// Bundled templates from embed.FS are loaded first, then local overrides
// from .plan-bender/templates/ replace matching filenames.
func LoadTemplates(projectRoot string) (map[string]string, error) {
	templates := make(map[string]string)

	// Load embedded templates
	entries, err := fs.ReadDir(embeddedFS, "embedded")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tmpl") {
			continue
		}
		data, err := fs.ReadFile(embeddedFS, "embedded/"+e.Name())
		if err != nil {
			return nil, err
		}
		templates[e.Name()] = string(data)
	}

	// Load local overrides
	overrideDir := filepath.Join(projectRoot, ".plan-bender", "templates")
	entries2, err := os.ReadDir(overrideDir)
	if err != nil {
		if os.IsNotExist(err) {
			return templates, nil
		}
		return nil, err
	}
	for _, e := range entries2 {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tmpl") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(overrideDir, e.Name()))
		if err != nil {
			return nil, err
		}
		templates[e.Name()] = string(data)
	}

	return templates, nil
}
