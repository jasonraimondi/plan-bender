package planrepo

import (
	"errors"
	"io/fs"
	"sort"
	"strings"
)

// PlanSummary is a lightweight per-plan summary used by List.
type PlanSummary struct {
	Slug    string
	Name    string
	Status  string
	Issues  int
	Done    int
	Points  int
	Blocked int
}

// List returns best-effort summaries of all plans in plansDir. Plans whose
// PRD or issues fail to parse are skipped, not surfaced as errors. Returns
// an empty slice when plansDir does not exist.
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
			continue
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
