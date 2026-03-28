package cli

import (
	"fmt"
	"os"
	"strings"

	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"sort"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var statusColors = map[string]string{
	"done":        "#2da44e",
	"in-progress": "#bf8700",
	"in-review":   "#bf8700",
	"blocked":     "#cf222e",
	"backlog":     "#656d76",
	"todo":        "#656d76",
	"canceled":    "#656d76",
	"qa":          "#8250df",
}

// NewGraphCmd creates the graph command.
func NewGraphCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "graph <slug>",
		Short: "Display issue dependency graph as Mermaid",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			issues, err := loadIssues(cfg.PlansDir, slug)
			if err != nil {
				return err
			}

			mermaid := buildMermaidGraph(issues)
			fmt.Fprint(cmd.OutOrStdout(), mermaid)
			return nil
		},
	}
}

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

func buildMermaidGraph(issues []schema.IssueYaml) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	// Nodes
	for _, iss := range issues {
		nodeID := fmt.Sprintf("i%d", iss.ID)
		label := fmt.Sprintf("#%d %s", iss.ID, iss.Name)
		b.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nodeID, label))
	}

	b.WriteString("\n")

	// Edges from blocked_by
	for _, iss := range issues {
		for _, dep := range iss.BlockedBy {
			b.WriteString(fmt.Sprintf("    i%d --> i%d\n", dep, iss.ID))
		}
	}

	b.WriteString("\n")

	// Style directives
	for _, iss := range issues {
		color, ok := statusColors[iss.Status]
		if !ok {
			color = "#656d76"
		}
		nodeID := fmt.Sprintf("i%d", iss.ID)
		b.WriteString(fmt.Sprintf("    style %s fill:%s,color:#fff\n", nodeID, color))
	}

	return b.String()
}
