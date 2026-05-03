package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the `status` command, a read-only per-issue view of a
// plan's state. Pairs with `retry` for recovering from blocked dispatches —
// status surfaces the blocked reason, retry clears it.
func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <slug>",
		Short: "Show per-issue state for a plan: status, blocked reason, branch, labels",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return NewAgentError("config load failed: "+err.Error(), ErrConfigError)
			}

			repo := planrepo.NewProd(cfg.PlansDir)
			sess, err := repo.Open(slug)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return NewAgentError(fmt.Sprintf("plan %q not found: %s", slug, err), ErrPlanNotFound)
				}
				return NewAgentError("opening plan: "+err.Error(), ErrInternal)
			}
			defer func() { _ = sess.Close() }()

			snap := sess.Snapshot()
			prd := snap.PRD
			issues := append([]schema.IssueYaml(nil), snap.Issues...)
			sort.SliceStable(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })

			counts, order := countByStatus(issues)

			if isAgentMode(cmd) {
				return writeStatusJSON(cmd.OutOrStdout(), &prd, issues, counts, order)
			}
			writeStatusHuman(cmd.OutOrStdout(), &prd, issues, counts, order)
			return nil
		},
	}
}

// statusJSON is the wire shape for `pba status`. Field order is stable.
type statusJSON struct {
	Plan   planSummaryJSON   `json:"plan"`
	Issues []issueSummaryJSON `json:"issues"`
}

type planSummaryJSON struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Total    int            `json:"total"`
	ByStatus map[string]int `json:"by_status"`
}

type issueSummaryJSON struct {
	ID        int      `json:"id"`
	Slug      string   `json:"slug"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Priority  string   `json:"priority"`
	Points    int      `json:"points"`
	Track     string   `json:"track"`
	Labels    []string `json:"labels"`
	BlockedBy []int    `json:"blocked_by"`
	Branch    *string  `json:"branch"`
	PR        *string  `json:"pr"`
	Notes     *string  `json:"notes"`
}

func writeStatusJSON(w io.Writer, prd *schema.PrdYaml, issues []schema.IssueYaml, counts map[string]int, _ []string) error {
	out := statusJSON{
		Plan: planSummaryJSON{
			Slug:     prd.Slug,
			Name:     prd.Name,
			Total:    len(issues),
			ByStatus: counts,
		},
		Issues: make([]issueSummaryJSON, 0, len(issues)),
	}
	for _, iss := range issues {
		labels := iss.Labels
		if labels == nil {
			labels = []string{}
		}
		blockedBy := iss.BlockedBy
		if blockedBy == nil {
			blockedBy = []int{}
		}
		out.Issues = append(out.Issues, issueSummaryJSON{
			ID:        iss.ID,
			Slug:      iss.Slug,
			Name:      iss.Name,
			Status:    iss.Status,
			Priority:  iss.Priority,
			Points:    iss.Points,
			Track:     iss.Track,
			Labels:    labels,
			BlockedBy: blockedBy,
			Branch:    iss.Branch,
			PR:        iss.PR,
			Notes:     iss.Notes,
		})
	}
	return json.NewEncoder(w).Encode(out)
}

func writeStatusHuman(w io.Writer, prd *schema.PrdYaml, issues []schema.IssueYaml, counts map[string]int, order []string) {
	fmt.Fprintf(w, "Plan: %s", prd.Slug)
	if prd.Name != "" && prd.Name != prd.Slug {
		fmt.Fprintf(w, " (%s)", prd.Name)
	}
	fmt.Fprintln(w)

	parts := make([]string, 0, len(order))
	for _, s := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
	}
	if len(parts) == 0 {
		fmt.Fprintf(w, "%d issues\n", len(issues))
	} else {
		fmt.Fprintf(w, "%d issues — %s\n", len(issues), strings.Join(parts, " · "))
	}
	if len(issues) == 0 {
		return
	}
	fmt.Fprintln(w)

	statusWidth := 0
	for _, iss := range issues {
		if l := len(iss.Status); l > statusWidth {
			statusWidth = l
		}
	}

	for _, iss := range issues {
		flags := ""
		if len(iss.Labels) > 0 {
			flags = "  [" + strings.Join(iss.Labels, ",") + "]"
		}
		dep := ""
		if len(iss.BlockedBy) > 0 {
			ids := make([]string, len(iss.BlockedBy))
			for i, id := range iss.BlockedBy {
				ids[i] = fmt.Sprintf("#%d", id)
			}
			dep = "  blocked_by: " + strings.Join(ids, ",")
		}
		fmt.Fprintf(w, "  #%-3d  %-*s  %s%s%s\n", iss.ID, statusWidth, iss.Status, iss.Slug, flags, dep)
		if iss.Branch != nil && *iss.Branch != "" {
			fmt.Fprintf(w, "        branch: %s\n", *iss.Branch)
		}
		if iss.PR != nil && *iss.PR != "" {
			fmt.Fprintf(w, "        pr: %s\n", *iss.PR)
		}
		if iss.Notes != nil && *iss.Notes != "" {
			fmt.Fprintf(w, "        notes: %s\n", firstLine(*iss.Notes))
		}
	}
}

// countByStatus tallies issues per status and returns a stable display order:
// statuses observed in this plan, with a canonical ordering for the common
// states. Unknown statuses are appended in the order first seen.
func countByStatus(issues []schema.IssueYaml) (map[string]int, []string) {
	counts := make(map[string]int, 8)
	seen := make([]string, 0, 8)
	for _, iss := range issues {
		if _, ok := counts[iss.Status]; !ok {
			seen = append(seen, iss.Status)
		}
		counts[iss.Status]++
	}

	canonical := []string{"backlog", "todo", "in-progress", "blocked", "in-review", "qa", "done", "canceled"}
	rank := make(map[string]int, len(canonical))
	for i, s := range canonical {
		rank[s] = i
	}
	sort.SliceStable(seen, func(i, j int) bool {
		ri, oi := rank[seen[i]]
		rj, oj := rank[seen[j]]
		if oi && oj {
			return ri < rj
		}
		if oi {
			return true
		}
		if oj {
			return false
		}
		return seen[i] < seen[j]
	})
	return counts, seen
}

// firstLine returns s up to the first newline. The dispatcher concatenates
// failure reasons with "\n\n" and tools may inject long stderr; humans want
// the headline, agents get the full string in JSON.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
