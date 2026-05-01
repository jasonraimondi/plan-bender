package plan

import (
	"fmt"
	"sort"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Status values referenced by the resolver. Mirrors workflow_states in config.
const (
	statusInProgress = "in-progress"
	statusTodo       = "todo"
	statusBacklog    = "backlog"
	statusBlocked    = "blocked"
	statusDone       = "done"
	statusCanceled   = "canceled"
)

// Result is the output of Resolve. Issue is nil when no candidate is ready.
type Result struct {
	Issue         *schema.IssueYaml `json:"issue"`
	Reason        string            `json:"reason"`
	WasBlocked    bool              `json:"was_blocked"`
	RequiresHuman bool              `json:"requires_human"`
	AllDone       bool              `json:"all_done"`
	BlockedCount  int               `json:"blocked_count"`
	Skipped       []SkippedIssue    `json:"skipped"`
}

// SkippedIssue is one entry in Result.Skipped — every issue not chosen with a one-line reason.
type SkippedIssue struct {
	ID     int    `json:"id"`
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

// candidate is an internal pairing of an issue with its derived "stale-blocked" flag.
type candidate struct {
	issue      *schema.IssueYaml
	wasBlocked bool
}

// ReadyAFK returns every AFK-labeled issue that has no open blockers and is in
// a non-terminal status (not done, canceled, or in-review). This is the parallel
// batch input for Dispatcher.RunBatch — order is by issue ID for stability.
func ReadyAFK(issues []schema.IssueYaml) []schema.IssueYaml {
	byID := make(map[int]*schema.IssueYaml, len(issues))
	for i := range issues {
		byID[issues[i].ID] = &issues[i]
	}

	var ready []schema.IssueYaml
	for i := range issues {
		iss := issues[i]
		switch iss.Status {
		case statusDone, statusCanceled, statusBlocked, "in-review":
			continue
		}
		if iss.Assignee != nil && *iss.Assignee != "" {
			continue
		}
		if !hasLabel(iss.Labels, "AFK") {
			continue
		}
		if len(openBlockers(iss.BlockedBy, byID)) > 0 {
			continue
		}
		ready = append(ready, iss)
	}

	sort.SliceStable(ready, func(i, j int) bool { return ready[i].ID < ready[j].ID })
	return ready
}

// Resolve picks the single recommended next issue from a plan's issues.
// Pure read — no I/O, no mutation. See PRD next-issue-resolver for ordering rules.
func Resolve(issues []schema.IssueYaml) Result {
	byID := make(map[int]*schema.IssueYaml, len(issues))
	for i := range issues {
		byID[issues[i].ID] = &issues[i]
	}

	unblocksCount := make(map[int]int, len(issues))
	for _, iss := range issues {
		for _, dep := range iss.BlockedBy {
			unblocksCount[dep]++
		}
	}

	var candidates []candidate
	var skipped []SkippedIssue
	allDone := true
	blockedCount := 0

	for i := range issues {
		iss := &issues[i]

		switch iss.Status {
		case statusDone, statusCanceled:
			skipped = append(skipped, SkippedIssue{
				ID:     iss.ID,
				Slug:   iss.Slug,
				Reason: fmt.Sprintf("status %s", iss.Status),
			})
			continue
		}
		allDone = false

		if iss.Assignee != nil && *iss.Assignee != "" {
			skipped = append(skipped, SkippedIssue{
				ID:     iss.ID,
				Slug:   iss.Slug,
				Reason: fmt.Sprintf("assigned to %s", *iss.Assignee),
			})
			continue
		}

		open := openBlockers(iss.BlockedBy, byID)
		if len(open) > 0 {
			if iss.Status == statusBlocked {
				blockedCount++
			}
			skipped = append(skipped, SkippedIssue{
				ID:     iss.ID,
				Slug:   iss.Slug,
				Reason: fmt.Sprintf("blocked_by %v", open),
			})
			continue
		}

		switch iss.Status {
		case statusInProgress, statusTodo, statusBacklog:
			candidates = append(candidates, candidate{issue: iss})
		case statusBlocked:
			candidates = append(candidates, candidate{issue: iss, wasBlocked: true})
		default:
			skipped = append(skipped, SkippedIssue{
				ID:     iss.ID,
				Slug:   iss.Slug,
				Reason: fmt.Sprintf("status %s not in candidate pool", iss.Status),
			})
		}
	}

	hasAFK := false
	for _, c := range candidates {
		if hasLabel(c.issue.Labels, "AFK") {
			hasAFK = true
			break
		}
	}

	pool := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if hasAFK && hasLabel(c.issue.Labels, "HITL") && !hasLabel(c.issue.Labels, "AFK") {
			skipped = append(skipped, SkippedIssue{
				ID:     c.issue.ID,
				Slug:   c.issue.Slug,
				Reason: "HITL — AFK pool not empty",
			})
			continue
		}
		pool = append(pool, c)
	}

	if len(pool) == 0 {
		return Result{
			AllDone:      allDone,
			BlockedCount: blockedCount,
			Skipped:      skipped,
		}
	}

	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		ar, br := statusRank(a), statusRank(b)
		if ar != br {
			return ar < br
		}
		ap, bp := priorityRank(a.issue.Priority), priorityRank(b.issue.Priority)
		if ap != bp {
			return ap < bp
		}
		au, bu := unblocksCount[a.issue.ID], unblocksCount[b.issue.ID]
		if au != bu {
			return au > bu
		}
		return a.issue.ID < b.issue.ID
	})

	chosen := pool[0]
	requiresHuman := !hasAFK && hasLabel(chosen.issue.Labels, "HITL")

	for _, c := range pool[1:] {
		skipped = append(skipped, SkippedIssue{
			ID:     c.issue.ID,
			Slug:   c.issue.Slug,
			Reason: notChosenReason(c, chosen, unblocksCount),
		})
	}

	return Result{
		Issue:         chosen.issue,
		Reason:        chosenReason(chosen, requiresHuman),
		WasBlocked:    chosen.wasBlocked,
		RequiresHuman: requiresHuman,
		AllDone:       false,
		BlockedCount:  blockedCount,
		Skipped:       skipped,
	}
}

// openBlockers returns the subset of dep IDs whose issue is not done|canceled.
// Unknown IDs count as open so dangling references don't silently mark an issue ready.
func openBlockers(deps []int, byID map[int]*schema.IssueYaml) []int {
	var open []int
	for _, id := range deps {
		dep, ok := byID[id]
		if !ok || (dep.Status != statusDone && dep.Status != statusCanceled) {
			open = append(open, id)
		}
	}
	return open
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func statusRank(c candidate) int {
	if c.wasBlocked {
		return 3
	}
	switch c.issue.Status {
	case statusInProgress:
		return 0
	case statusTodo:
		return 1
	case statusBacklog:
		return 2
	}
	return 4
}

func priorityRank(p string) int {
	switch p {
	case "urgent":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	}
	return 4
}

func chosenReason(c candidate, requiresHuman bool) string {
	parts := []string{fmt.Sprintf("status %s, priority %s", c.issue.Status, c.issue.Priority)}
	if c.wasBlocked {
		parts = append(parts, "stale-blocked (deps resolved)")
	}
	if requiresHuman {
		parts = append(parts, "HITL — requires human")
	}
	return joinReasons(parts)
}

func notChosenReason(c, chosen candidate, unblocksCount map[int]int) string {
	if statusRank(c) != statusRank(chosen) {
		return fmt.Sprintf("lower status rank (%s)", c.issue.Status)
	}
	if priorityRank(c.issue.Priority) != priorityRank(chosen.issue.Priority) {
		return fmt.Sprintf("lower priority (%s)", c.issue.Priority)
	}
	if unblocksCount[c.issue.ID] != unblocksCount[chosen.issue.ID] {
		return fmt.Sprintf("unblocks fewer issues (%d vs %d)",
			unblocksCount[c.issue.ID], unblocksCount[chosen.issue.ID])
	}
	return fmt.Sprintf("higher id than chosen (%d > %d)", c.issue.ID, chosen.issue.ID)
}

func joinReasons(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}
