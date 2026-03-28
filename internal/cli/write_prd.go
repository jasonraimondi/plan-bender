package cli

import (
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
		Use:   "write-prd [file]",
		Short: "Validate and write a PRD YAML file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()
			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			data, err := readInput(cmd, args)
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

			dir := filepath.Join(cfg.PlansDir, prd.Slug)
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

			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", outPath)
			return nil
		},
	}
}

func readInput(cmd *cobra.Command, args []string) ([]byte, error) {
	if len(args) > 0 {
		return os.ReadFile(args[0])
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
