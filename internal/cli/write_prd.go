package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

			dir := filepath.Join(cfg.PlansDir, slug)
			if err := os.MkdirAll(filepath.Join(dir, "issues"), 0o755); err != nil {
				return err
			}

			outPath := filepath.Join(dir, "prd.yaml")
			outData, err := yaml.Marshal(&prd)
			if err != nil {
				return err
			}

			if err := atomicWriteFile(outPath, outData, 0o644); err != nil {
				return err
			}

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

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pb-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
