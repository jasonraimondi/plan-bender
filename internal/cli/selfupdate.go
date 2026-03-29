package cli

import (
	"fmt"
	"net/http"

	"github.com/jasonraimondi/plan-bender/internal/update"
	"github.com/spf13/cobra"
)

type selfUpdateCmd struct {
	version             string
	checkForUpdate      func(currentVersion string) (latest string, isNewer bool, err error)
	detectInstallMethod func() (update.InstallMethod, error)
}

// NewSelfUpdateCmd creates the self-update command.
func NewSelfUpdateCmd(version string) *cobra.Command {
	sc := &selfUpdateCmd{
		version: version,
		checkForUpdate: func(currentVersion string) (string, bool, error) {
			return update.CheckForUpdate(currentVersion, &http.Client{})
		},
		detectInstallMethod: func() (update.InstallMethod, error) {
			return update.DetectCurrentInstallMethod()
		},
	}

	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update plan-bender to the latest version",
		Args:  cobra.NoArgs,
		RunE:  sc.run,
	}

	selfUpdateRegistry[cmd] = sc
	return cmd
}

// selfUpdateRegistry allows tests to access the selfUpdateCmd for dependency injection.
var selfUpdateRegistry = map[*cobra.Command]*selfUpdateCmd{}

// selfUpdateFromCmd retrieves the selfUpdateCmd associated with a cobra.Command.
func selfUpdateFromCmd(cmd *cobra.Command) *selfUpdateCmd {
	return selfUpdateRegistry[cmd]
}

func (sc *selfUpdateCmd) run(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	if sc.version == "dev" {
		fmt.Fprintln(out, "Skipping update check for development build")
		return nil
	}

	latest, isNewer, err := sc.checkForUpdate(sc.version)
	if err != nil {
		return fmt.Errorf("checking for update: %w", err)
	}

	if !isNewer {
		fmt.Fprintf(out, "plan-bender is up to date (v%s)\n", sc.version)
		return nil
	}

	method, err := sc.detectInstallMethod()
	if err != nil {
		return fmt.Errorf("detecting install method: %w", err)
	}

	switch method {
	case update.InstallMethodNPM:
		fmt.Fprintf(out, "A newer version is available: %s → %s\n", sc.version, latest)
		fmt.Fprintln(out, "  Run: npm install -g @jasonraimondi/plan-bender@latest")
	case update.InstallMethodDirect:
		fmt.Fprintf(out, "Updating plan-bender to v%s...\n", latest)
	}

	return nil
}
