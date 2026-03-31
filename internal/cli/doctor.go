package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/agents"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/spf13/cobra"
)

// CheckResult holds the outcome of a single doctor check.
type CheckResult struct {
	Name    string
	Pass    bool
	Message string
}

// RunChecks runs all doctor checks and returns the results.
func RunChecks(root string, cfg config.Config, version string) []CheckResult {
	results := []CheckResult{
		configCheck(root),
		skillsCheck(root, cfg),
		plansDirCheck(root, cfg),
		versionCheck(version),
		linearCheck(cfg),
	}
	return results
}

// NewDoctorCmd creates the doctor command.
func NewDoctorCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Check installation health",
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cfg, loadErr := config.Load(root)
			if loadErr != nil {
				// Still run checks with defaults so we get a useful report
				cfg = config.Defaults()
			}

			results := RunChecks(root, cfg, version)

			// Override the config check result if loading actually failed
			if loadErr != nil {
				results[0] = CheckResult{
					Name:    "config",
					Pass:    false,
					Message: loadErr.Error(),
				}
			}

			failed := false
			for _, r := range results {
				if r.Pass {
					line := fmt.Sprintf("\u2713 %s", r.Name)
					if r.Message != "" {
						line += " \u2014 " + r.Message
					}
					fmt.Fprintln(cmd.OutOrStdout(), line)
				} else {
					failed = true
					fmt.Fprintf(cmd.OutOrStdout(), "\u2717 %s \u2014 %s\n", r.Name, r.Message)
				}
			}

			if failed {
				os.Exit(1)
			}
			return nil
		},
	}
	return cmd
}

func configCheck(root string) CheckResult {
	_, err := config.Load(root)
	if err != nil {
		return CheckResult{Name: "config", Pass: false, Message: err.Error()}
	}
	return CheckResult{Name: "config", Pass: true, Message: "valid"}
}

func skillsCheck(root string, cfg config.Config) CheckResult {
	totalSkills := 0
	totalSymlinks := 0

	for _, agentName := range cfg.Agents {
		ac, err := agents.Get(agentName)
		if err != nil {
			return CheckResult{Name: "skills", Pass: false, Message: fmt.Sprintf("unknown agent %q", agentName)}
		}

		sourceDir := filepath.Join(root, ".plan-bender", "skills", agentName)
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return CheckResult{Name: "skills", Pass: false, Message: fmt.Sprintf("skills not generated for %s", agentName)}
		}

		skillDirs := 0
		for _, e := range entries {
			if e.IsDir() {
				skillDirs++
			}
		}
		totalSkills += skillDirs

		targetDir, err := resolveAgentDir(root, ac)
		if err != nil {
			return CheckResult{Name: "skills", Pass: false, Message: fmt.Sprintf("cannot resolve target dir for %s: %s", agentName, err)}
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dst := filepath.Join(targetDir, e.Name())
			info, err := os.Lstat(dst)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				return CheckResult{
					Name:    "skills",
					Pass:    false,
					Message: fmt.Sprintf("symlink missing: %s in %s", e.Name(), agentName),
				}
			}
			totalSymlinks++
		}
	}

	if totalSkills == 0 {
		return CheckResult{Name: "skills", Pass: false, Message: "no skills generated"}
	}
	return CheckResult{Name: "skills", Pass: true, Message: fmt.Sprintf("%d skills, %d symlinks", totalSkills, totalSymlinks)}
}

func plansDirCheck(root string, cfg config.Config) CheckResult {
	plansDir := cfg.PlansDir
	if !filepath.IsAbs(plansDir) {
		plansDir = filepath.Join(root, plansDir)
	}

	info, err := os.Stat(plansDir)
	if err != nil {
		return CheckResult{Name: "plans dir", Pass: false, Message: fmt.Sprintf("%s does not exist", cfg.PlansDir)}
	}
	if !info.IsDir() {
		return CheckResult{Name: "plans dir", Pass: false, Message: fmt.Sprintf("%s is not a directory", cfg.PlansDir)}
	}
	return CheckResult{Name: "plans dir", Pass: true, Message: cfg.PlansDir}
}

func versionCheck(version string) CheckResult {
	path, err := exec.LookPath("plan-bender-agent")
	if err != nil {
		return CheckResult{
			Name:    "versions",
			Pass:    false,
			Message: "plan-bender-agent not found — install plan-bender-agent or check PATH",
		}
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return CheckResult{Name: "versions", Pass: false, Message: fmt.Sprintf("failed to get agent version: %s", err)}
	}

	agentVersion := strings.TrimSpace(string(out))
	// The --version output is typically "plan-bender-agent version X.Y.Z"
	agentVersion = strings.TrimPrefix(agentVersion, "plan-bender-agent version ")

	if agentVersion != version {
		return CheckResult{
			Name:    "versions",
			Pass:    false,
			Message: fmt.Sprintf("mismatch: pb=%s, pba=%s", version, agentVersion),
		}
	}
	return CheckResult{Name: "versions", Pass: true, Message: version}
}

func linearCheck(cfg config.Config) CheckResult {
	if !cfg.Linear.Enabled {
		return CheckResult{Name: "linear", Pass: true, Message: "skipped \u2014 not enabled"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := linear.NewClient(cfg.Linear.APIKey)
	_, err := client.ListWorkflowStates(ctx, cfg.Linear.Team)
	if err != nil {
		return CheckResult{Name: "linear", Pass: false, Message: fmt.Sprintf("API unreachable: %s", err)}
	}
	return CheckResult{Name: "linear", Pass: true, Message: "API reachable"}
}
