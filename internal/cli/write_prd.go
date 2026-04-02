package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewWritePrdCmd creates the write-prd command.
func NewWritePrdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "write-prd <slug> [file]",
		Short: "Validate and write a PRD YAML file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			slug := args[0]

			data, err := readInput(cmd, args[1:])
			if err != nil {
				return err
			}

			var prd schema.PrdYaml
			if err := yaml.Unmarshal(data, &prd); err != nil {
				return fmt.Errorf("invalid YAML: %w", err)
			}

			errs := prd.Validate()
			if len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", e)
				}
				return fmt.Errorf("validation failed")
			}

			store := backend.NewProdPlanStore(cfg.PlansDir)
			if err := store.WritePrd(slug, &prd); err != nil {
				return err
			}

			outPath := filepath.Join(cfg.PlansDir, slug, "prd.yaml")
			if isAgentMode(cmd) {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{
					"status": "ok",
					"file":   outPath,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}
}

func readInput(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) > 0 {
		return os.ReadFile(args[0])
	}
	if f, ok := cmd.InOrStdin().(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return nil, fmt.Errorf("no input — pipe YAML or pass a file path")
		}
	}
	return io.ReadAll(cmd.InOrStdin())
}

