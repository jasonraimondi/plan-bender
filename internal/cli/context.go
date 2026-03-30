package cli

import (
	"encoding/json"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
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

			if len(args) == 0 {
				return contextList(cmd, cfg)
			}
			return contextDetail(cmd, cfg, args[0])
		},
	}
}

func contextList(cmd *cobra.Command, cfg config.Config) error {
	summaries, err := plan.ListPlans(cfg.PlansDir)
	if err != nil {
		return NewAgentError("listing plans: "+err.Error(), ErrInternal)
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(summaries)
}

func contextDetail(cmd *cobra.Command, cfg config.Config, slug string) error {
	prd, err := plan.LoadPrd(cfg.PlansDir, slug)
	if err != nil {
		return NewAgentError("plan not found: "+slug, ErrPlanNotFound)
	}

	issues, err := plan.LoadIssues(cfg.PlansDir, slug)
	if err != nil {
		return NewAgentError("loading issues: "+err.Error(), ErrInternal)
	}

	ctx := contextFullJSON{
		Prd:          prd,
		Issues:       issues,
		Dependencies: plan.BuildGraphJSON(issues),
		Stats:        plan.IssueStats(issues),
	}

	return json.NewEncoder(cmd.OutOrStdout()).Encode(ctx)
}
