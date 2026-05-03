package planrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// PlanSummary is a lightweight per-plan summary used by List.
type PlanSummary struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Issues  int    `json:"issues"`
	Done    int    `json:"done"`
	Points  int    `json:"points"`
	Blocked int    `json:"blocked"`
}

// List returns summaries of all plans in plansDir. Returns an empty slice
// when plansDir does not exist. A plan whose PRD or issues fail to load makes
// the listing fail so corrupt or half-written plans are visible to callers.
//
// Summaries are sorted by slug so output order is deterministic.
func (p *Plans) List() ([]PlanSummary, error) {
	entries, err := fs.ReadDir(p.adapters.FS, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []PlanSummary{}, nil
		}
		return nil, err
	}

	summaries := make([]PlanSummary, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		snap, err := loadSnapshot(p.adapters.FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("loading plan %q: %w", e.Name(), err)
		}
		summaries = append(summaries, summarize(snap))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Slug < summaries[j].Slug
	})
	return summaries, nil
}

func summarize(s *Snapshot) PlanSummary {
	sum := PlanSummary{
		Slug:   s.Slug,
		Name:   s.PRD.Name,
		Status: s.PRD.Status,
		Issues: len(s.Issues),
	}
	for _, iss := range s.Issues {
		sum.Points += iss.Points
		switch iss.Status {
		case "done":
			sum.Done++
		case "blocked":
			sum.Blocked++
		}
	}
	return sum
}
