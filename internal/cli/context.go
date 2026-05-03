package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
)

type contextFullJSON struct {
	Prd          *schema.PrdYaml    `json:"prd"`
	Issues       []schema.IssueYaml `json:"issues"`
	Dependencies plan.Graph         `json:"dependencies"`
	Stats        plan.Stats         `json:"stats"`
}

// NewContextCmd creates the context command for the agent binary.
// No slug: returns JSON array of plan summaries.
// With slug: returns full plan context as JSON.
func NewContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context [slug]",
		Short: "Return plan context as JSON",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return NewAgentError("config: "+err.Error(), ErrConfigError)
			}

			repo := planrepo.NewProd(cfg.PlansDir)
			if len(args) == 0 {
				return contextList(cmd, repo)
			}
			return contextDetail(cmd, repo, args[0])
		},
	}
}

func contextList(cmd *cobra.Command, repo *planrepo.Plans) error {
	summaries, err := repo.List()
	if err != nil {
		return NewAgentError("listing plans: "+err.Error(), ErrInternal)
	}
	if summaries == nil {
		summaries = []planrepo.PlanSummary{}
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(summaries)
}

func contextDetail(cmd *cobra.Command, repo *planrepo.Plans, slug string) error {
	sess, err := repo.Open(slug)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewAgentError("plan not found: "+slug, ErrPlanNotFound)
		}
		return NewAgentError("opening plan: "+err.Error(), ErrInternal)
	}
	defer func() { _ = sess.Close() }()

	snap, err := sess.Snapshot()
	if err != nil {
		return NewAgentError("reading snapshot: "+err.Error(), ErrInternal)
	}
	prd := snap.PRD
	ctx := contextFullJSON{
		Prd:          &prd,
		Issues:       snap.Issues,
		Dependencies: plan.BuildGraphJSON(snap.Issues),
		Stats:        plan.IssueStats(snap.Issues),
	}

	return json.NewEncoder(cmd.OutOrStdout()).Encode(ctx)
}
