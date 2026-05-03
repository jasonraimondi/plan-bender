package cli

import (
	"encoding/json"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
)

type agentValidationError struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

type agentValidationResult struct {
	Valid  bool                   `json:"valid"`
	Errors []agentValidationError `json:"errors"`
}

// NewAgentValidateCmd creates the agent validate subcommand.
func NewAgentValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <slug>",
		Short: "Validate a plan and return structured JSON errors",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]

			cfg, err := config.Load(".")
			if err != nil {
				return NewAgentError("config load failed: "+err.Error(), ErrConfigError)
			}

			repo := planrepo.NewProd(cfg.PlansDir)
			planResult, err := repo.Validate(slug, cfg)
			if err != nil {
				return NewAgentError("opening plan "+slug+": "+err.Error(), ErrPlanNotFound)
			}

			out := transformValidationResult(planResult)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
}

func transformValidationResult(r schema.PlanValidationResult) agentValidationResult {
	var errs []agentValidationError

	for _, msg := range r.PRD.Errors {
		errs = append(errs, agentValidationError{
			Severity: "error",
			File:     r.PRD.File,
			Field:    "prd",
			Message:  msg,
		})
	}

	for _, ir := range r.Issues {
		for _, msg := range ir.Errors {
			errs = append(errs, agentValidationError{
				Severity: "error",
				File:     ir.File,
				Field:    "issue",
				Message:  msg,
			})
		}
	}

	for _, msg := range r.CrossRef {
		errs = append(errs, agentValidationError{
			Severity: "error",
			File:     "",
			Field:    "cross_ref",
			Message:  msg,
		})
	}

	for _, msg := range r.Cycles {
		errs = append(errs, agentValidationError{
			Severity: "error",
			File:     "",
			Field:    "cycle",
			Message:  msg,
		})
	}

	if errs == nil {
		errs = []agentValidationError{}
	}

	return agentValidationResult{
		Valid:  r.Valid,
		Errors: errs,
	}
}
