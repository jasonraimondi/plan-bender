package cli

import (
	"encoding/json"
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
)

// NewValidateCmd creates the validate command.
func NewValidateCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "validate <slug>",
		Short: "Validate PRD and issue YAML files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]

			cfg, err := config.Load(".")
			if err != nil {
				return err
			}

			repo := planrepo.NewProd(cfg.PlansDir)
			result, err := repo.Validate(slug, cfg)
			if err != nil {
				return fmt.Errorf("opening plan %q: %w", slug, err)
			}

			if jsonOutput || isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}

			printValidationResult(cmd, result)

			if !result.Valid {
				return fmt.Errorf("validation failed")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

func printValidationResult(cmd *cobra.Command, result schema.PlanValidationResult) {
	out := cmd.OutOrStdout()

	if len(result.PRD.Errors) > 0 {
		fmt.Fprintf(out, "PRD (%s):\n", result.PRD.File)
		for _, e := range result.PRD.Errors {
			fmt.Fprintf(out, "  - %s\n", e)
		}
	}

	for _, ir := range result.Issues {
		if len(ir.Errors) > 0 {
			fmt.Fprintf(out, "Issue (%s):\n", ir.File)
			for _, e := range ir.Errors {
				fmt.Fprintf(out, "  - %s\n", e)
			}
		}
	}

	if len(result.CrossRef) > 0 {
		fmt.Fprintln(out, "Cross-reference errors:")
		for _, e := range result.CrossRef {
			fmt.Fprintf(out, "  - %s\n", e)
		}
	}

	if len(result.Cycles) > 0 {
		fmt.Fprintln(out, "Dependency cycles:")
		for _, e := range result.Cycles {
			fmt.Fprintf(out, "  - %s\n", e)
		}
	}

	if result.Valid {
		fmt.Fprintln(out, "Validation passed")
	}
}
