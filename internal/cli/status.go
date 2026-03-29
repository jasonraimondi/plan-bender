package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type planSummaryJSON struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Issues  int    `json:"issues"`
	Done    int    `json:"done"`
	Points  int    `json:"points"`
	Blocked int    `json:"blocked"`
}

type planDetailJSON struct {
	Slug   string             `json:"slug"`
	Name   string             `json:"name"`
	Status string             `json:"status"`
	Prd    *schema.PrdYaml    `json:"prd"`
	Issues []schema.IssueYaml `json:"issues"`
}

// NewStatusCmd creates the status command.
func NewStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status [slug]",
		Short: "Show plan status dashboard",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				return showPlanDetail(cmd, cfg, args[0], jsonOutput)
			}
			return showAllPlans(cmd, cfg, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func showAllPlans(cmd *cobra.Command, cfg config.Config, jsonOut bool) error {
	entries, err := os.ReadDir(cfg.PlansDir)
	if err != nil {
		if os.IsNotExist(err) {
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode([]planSummaryJSON{})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "No plans found")
			return nil
		}
		return err
	}

	out := cmd.OutOrStdout()
	var summaries []planSummaryJSON
	found := false
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		prdPath := filepath.Join(cfg.PlansDir, e.Name(), "prd.yaml")
		data, err := os.ReadFile(prdPath)
		if err != nil {
			continue
		}
		var prd schema.PrdYaml
		if err := yaml.Unmarshal(data, &prd); err != nil {
			continue
		}

		issues, _ := loadIssues(cfg.PlansDir, e.Name())
		done, total, donePoints, totalPoints, blocked := issueStats(issues)

		if jsonOut {
			summaries = append(summaries, planSummaryJSON{
				Slug:    e.Name(),
				Name:    prd.Name,
				Status:  prd.Status,
				Issues:  total,
				Done:    done,
				Points:  totalPoints,
				Blocked: blocked,
			})
		} else {
			fmt.Fprintf(out, "%s [%s] — %d/%d issues, %d/%d pts",
				prd.Name, prd.Status, done, total, donePoints, totalPoints)
			if blocked > 0 {
				fmt.Fprintf(out, " (%d blocked)", blocked)
			}
			fmt.Fprintln(out)
		}
		found = true
	}

	if jsonOut {
		if summaries == nil {
			summaries = []planSummaryJSON{}
		}
		return json.NewEncoder(out).Encode(summaries)
	}

	if !found {
		fmt.Fprintln(out, "No plans found")
	}
	return nil
}

func showPlanDetail(cmd *cobra.Command, cfg config.Config, slug string, jsonOut bool) error {
	out := cmd.OutOrStdout()

	prdPath := filepath.Join(cfg.PlansDir, slug, "prd.yaml")
	data, err := os.ReadFile(prdPath)
	if err != nil {
		return fmt.Errorf("reading PRD: %w", err)
	}
	var prd schema.PrdYaml
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return fmt.Errorf("parsing PRD: %w", err)
	}

	issues, err := loadIssues(cfg.PlansDir, slug)
	if err != nil {
		return err
	}

	if jsonOut {
		return json.NewEncoder(out).Encode(planDetailJSON{
			Slug:   slug,
			Name:   prd.Name,
			Status: prd.Status,
			Prd:    &prd,
			Issues: issues,
		})
	}

	done, total, donePoints, totalPoints, _ := issueStats(issues)
	fmt.Fprintf(out, "PRD: %s [%s]\n", prd.Name, prd.Status)
	fmt.Fprintf(out, "Issues: %d/%d done | Points: %d/%d\n\n", done, total, donePoints, totalPoints)

	// Per-issue listing
	for _, iss := range issues {
		blockedStr := ""
		if len(iss.BlockedBy) > 0 {
			ids := make([]string, len(iss.BlockedBy))
			for i, id := range iss.BlockedBy {
				ids[i] = fmt.Sprintf("#%d", id)
			}
			blockedStr = fmt.Sprintf(" blocked-by: %s", strings.Join(ids, ","))
		}
		fmt.Fprintf(out, "  #%d [%s] %s (%s, %dpt)%s\n",
			iss.ID, iss.Status, iss.Name, iss.Track, iss.Points, blockedStr)
	}

	// Per-track breakdown
	fmt.Fprintln(out, "\nTrack coverage:")
	trackDone := make(map[string]int)
	trackTotal := make(map[string]int)
	for _, iss := range issues {
		trackTotal[iss.Track]++
		if iss.Status == "done" {
			trackDone[iss.Track]++
		}
	}
	for _, t := range cfg.Tracks {
		fmt.Fprintf(out, "  %s: %d/%d\n", t, trackDone[t], trackTotal[t])
	}

	return nil
}

func issueStats(issues []schema.IssueYaml) (done, total, donePoints, totalPoints, blocked int) {
	for _, iss := range issues {
		total++
		totalPoints += iss.Points
		switch iss.Status {
		case "done":
			done++
			donePoints += iss.Points
		case "blocked":
			blocked++
		}
	}
	return
}
