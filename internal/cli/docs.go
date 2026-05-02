package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

const repoURL = "https://github.com/jasonraimondi/plan-bender"

const fullReference = `# Full config reference (.plan-bender.yaml)
# Three layers, deep-merged — later wins:
#   ~/.config/plan-bender/defaults.yaml  (global)
#   .plan-bender.yaml                    (project, committed)
#   .plan-bender.local.yaml              (local, gitignored)

plans_dir: ./.plan-bender/plans/
max_points: 3                  # Cap per issue — forces thin slices
agents:
  claude-code: true            # claude-code | opencode | openclaw | pi

tracks:                        # Classify issue concerns; PRD-to-issues checks coverage
  - intent
  - experience
  - data
  - rules
  - resilience

workflow_states:
  - backlog
  - todo
  - in-progress
  - blocked
  - in-review
  - qa
  - done
  - canceled

pipeline:
  skip: []                     # Skill names to exclude, e.g. [bender-interview-me]

issue_schema:
  custom_fields: []            # Add required fields to every issue
  # - name: team
  #   type: enum               # string | number | boolean | enum
  #   required: true
  #   enum_values: [frontend, backend, platform]

review_with_user: false        # Insert a user review step before writing PRDs/issues

update_check: true             # Check for new releases on pb commands

manage_gitignore: true         # Let pb setup manage .gitignore entries

# Per-agent overrides (object form):
# agents:
#   claude-code:
#     project_dir: .claude/skills/
#     scope: project            # project | user
#   opencode: true
#   pi: false

# Linear integration — put credentials in .plan-bender.local.yaml
# linear:
#   enabled: false
#   api_key: $LINEAR_API_KEY   # $VAR and ${VAR} expanded at load time
#   team: $LINEAR_TEAM_ID
#   project_id: ""             # Optional — scope sync to a project
#   status_map:
#     in-progress: "In Progress"
#     in-review: "In Review"`

// docsOpener opens a URL in the browser. Swappable for tests.
var docsOpener = func(url string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("browser open not supported on %s", runtime.GOOS)
	}
	return exec.Command("open", url).Run()
}

// NewDocsCmd creates the docs command.
func NewDocsCmd() *cobra.Command {
	var full, printOnly bool

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Open docs or show config reference",
		Long: `Open the plan-bender GitHub repo in your browser, or print config reference.

  pb docs          Open GitHub repo in browser (macOS) or print URL
  pb docs --print  Print the repo URL without opening
  pb docs --full   Print full config reference with all options`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if full {
				fmt.Fprintln(out, fullReference)
				return nil
			}

			if printOnly {
				fmt.Fprintln(out, repoURL)
				return nil
			}

			if err := docsOpener(repoURL); err != nil {
				fmt.Fprintln(out, repoURL)
				return nil
			}
			fmt.Fprintf(out, "Opened %s\n", repoURL)
			return nil
		},
	}

	cmd.Flags().BoolVar(&full, "full", false, "Print full config reference")
	cmd.Flags().BoolVar(&printOnly, "print", false, "Print repo URL without opening browser")

	return cmd
}
