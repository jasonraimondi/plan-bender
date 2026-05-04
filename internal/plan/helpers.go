package plan

import "github.com/jasonraimondi/plan-bender/internal/schema"

// Stats holds aggregate statistics for a set of issues.
type Stats struct {
	Total       int `json:"total"`
	Done        int `json:"done"`
	TotalPoints int `json:"points"`
	DonePoints  int `json:"done_points"`
	Blocked     int `json:"blocked"`
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
