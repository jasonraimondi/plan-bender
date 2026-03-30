package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

func loadIssues(plansDir, slug string) ([]schema.IssueYaml, error) {
	dir := filepath.Join(plansDir, slug, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading issues: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var issues []schema.IssueYaml
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var issue schema.IssueYaml
		if err := yaml.Unmarshal(data, &issue); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}
