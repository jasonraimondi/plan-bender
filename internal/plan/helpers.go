package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// Stats holds aggregate statistics for a set of issues.
type Stats struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	TotalPoints int `json:"points"`
	DonePoints int `json:"done_points"`
	Blocked    int `json:"blocked"`
}

// GraphNode is a node in the dependency graph.
type GraphNode struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// GraphEdge is a directed edge in the dependency graph.
type GraphEdge struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Graph holds the dependency graph for a plan's issues.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// PlanSummary is a lightweight summary of a plan for listing.
type PlanSummary struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Issues  int    `json:"issues"`
	Done    int    `json:"done"`
	Points  int    `json:"points"`
	Blocked int    `json:"blocked"`
}

// LoadPrd reads and parses a PRD YAML file for the given slug.
func LoadPrd(plansDir, slug string) (*schema.PrdYaml, error) {
	prdPath := filepath.Join(plansDir, slug, "prd.yaml")
	data, err := os.ReadFile(prdPath)
	if err != nil {
		return nil, fmt.Errorf("reading PRD for %q: %w", slug, err)
	}
	var prd schema.PrdYaml
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("parsing PRD for %q: %w", slug, err)
	}
	return &prd, nil
}

// LoadIssues reads and parses all issue YAML files for the given slug.
func LoadIssues(plansDir, slug string) ([]schema.IssueYaml, error) {
	dir := filepath.Join(plansDir, slug, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading issues for %q: %w", slug, err)
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

// IssueStats computes aggregate statistics for a set of issues.
func IssueStats(issues []schema.IssueYaml) Stats {
	var s Stats
	for _, iss := range issues {
		s.Total++
		s.TotalPoints += iss.Points
		switch iss.Status {
		case "done":
			s.Done++
			s.DonePoints += iss.Points
		case "blocked":
			s.Blocked++
		}
	}
	return s
}

// BuildGraphJSON builds a dependency graph from a set of issues.
func BuildGraphJSON(issues []schema.IssueYaml) Graph {
	var nodes []GraphNode
	var edges []GraphEdge

	for _, iss := range issues {
		nodes = append(nodes, GraphNode{
			ID:     iss.ID,
			Name:   iss.Name,
			Status: iss.Status,
		})
		for _, dep := range iss.BlockedBy {
			edges = append(edges, GraphEdge{From: dep, To: iss.ID})
		}
	}

	if nodes == nil {
		nodes = []GraphNode{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}

	return Graph{Nodes: nodes, Edges: edges}
}

// ListPlans returns a summary of each plan in the plans directory.
func ListPlans(plansDir string) ([]PlanSummary, error) {
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PlanSummary{}, nil
		}
		return nil, err
	}

	var summaries []PlanSummary
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		prd, err := LoadPrd(plansDir, e.Name())
		if err != nil {
			continue
		}

		issues, _ := LoadIssues(plansDir, e.Name())
		stats := IssueStats(issues)

		summaries = append(summaries, PlanSummary{
			Slug:    e.Name(),
			Name:    prd.Name,
			Status:  prd.Status,
			Issues:  stats.Total,
			Done:    stats.Done,
			Points:  stats.TotalPoints,
			Blocked: stats.Blocked,
		})
	}

	if summaries == nil {
		summaries = []PlanSummary{}
	}
	return summaries, nil
}
