package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
)

// NewInstallCmd creates the install command.
func NewInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install generated skills via symlinks",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, _ := os.Getwd()

			cfg, err := config.Load(root)
			if err != nil {
				return err
			}

			sourceDir := filepath.Join(root, ".plan-bender", "skills")
			if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
				return fmt.Errorf("no skills found — run `plan-bender generate-skills` first")
			}

			targetDir, err := resolveTargetDir(root, cfg.InstallTarget)
			if err != nil {
				return err
			}

			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				return fmt.Errorf("creating target dir: %w", err)
			}

			entries, err := os.ReadDir(sourceDir)
			if err != nil {
				return fmt.Errorf("reading skills dir: %w", err)
			}

			count := 0
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				src := filepath.Join(sourceDir, e.Name())
				dst := filepath.Join(targetDir, e.Name())

				info, err := os.Lstat(dst)
				if err == nil {
					if info.Mode()&os.ModeSymlink != 0 {
						os.Remove(dst)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "skipping %s: not a symlink\n", dst)
						continue
					}
				}

				if err := os.Symlink(src, dst); err != nil {
					return fmt.Errorf("symlinking %s: %w", e.Name(), err)
				}
				count++
			}

			if cfg.InstallTarget == config.InstallTargetProject {
				ensureGitignore(root)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%d skills installed\n", count)
			return nil
		},
	}
}

func resolveTargetDir(root string, target config.InstallTarget) (string, error) {
	if target == config.InstallTargetProject {
		return filepath.Join(root, ".claude", "skills"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

func ensureGitignore(root string) {
	gitignorePath := filepath.Join(root, ".gitignore")
	entries := []string{".plan-bender/", ".claude/skills/bender-*", ".plan-bender.local.yaml"}

	existing, _ := os.ReadFile(gitignorePath)
	content := string(existing)

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return
	}

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += strings.Join(toAdd, "\n") + "\n"
	os.WriteFile(gitignorePath, []byte(content), 0o644)
}
