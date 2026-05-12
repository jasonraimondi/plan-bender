package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

const repoURL = "https://github.com/jasonraimondi/plan-bender"

const fullReference = `Full config reference (.plan-bender.json)

Three layers, deep-merged — later wins:
  ~/.config/plan-bender/defaults.json  (global)
  .plan-bender.json                    (project, committed)
  .plan-bender.local.json              (local, gitignored)

{
  "plans_dir": "./.plan-bender/plans/",
  "max_points": 3,
  "agents": {
    "claude-code": true
  },
  "tracks": ["intent", "experience", "data", "rules", "resilience"],
  "workflow_states": [
    "backlog", "todo", "in-progress", "blocked",
    "in-review", "qa", "done", "canceled"
  ],
  "pipeline": {
    "skip": []
  },
  "issue_schema": {
    "custom_fields": []
  },
  "review_with_user": false,
  "report_bugs": false,
  "update_check": true,
  "manage_gitignore": false
}

Per-agent overrides (object form):
  "agents": {
    "claude-code": {
      "project_dir": ".claude/skills/",
      "scope": "project"
    },
    "opencode": true,
    "pi": false
  }

Linear integration — put credentials in .plan-bender.local.json:
  "linear": {
    "enabled": true,
    "api_key": "$LINEAR_API_KEY",
    "team": "$LINEAR_TEAM_ID",
    "project_id": "",
    "status_map": {
      "in-progress": "In Progress",
      "in-review": "In Review"
    }
  }

issue_schema.custom_fields entry:
  { "name": "team",
    "type": "enum",
    "required": true,
    "enum_values": ["frontend", "backend", "platform"] }
`

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
				fmt.Fprint(out, fullReference)
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
