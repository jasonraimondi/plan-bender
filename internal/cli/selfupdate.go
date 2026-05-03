package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jasonraimondi/plan-bender/internal/update"
	"github.com/spf13/cobra"
)

type selfUpdateCmd struct {
	version             string
	checkForUpdate      func(currentVersion string) (latest string, isNewer bool, err error)
	detectInstallMethod func() (update.InstallMethod, error)
	downloadAndReplace  func(version string) error
	fetchReleaseNotes   func(version string) (string, error)
}

// NewSelfUpdateCmd creates the self-update command.
func NewSelfUpdateCmd(version string) *cobra.Command {
	client := &http.Client{}
	sc := &selfUpdateCmd{
		version: version,
		checkForUpdate: func(currentVersion string) (string, bool, error) {
			return update.CheckForUpdate(currentVersion, client, true)
		},
		detectInstallMethod: func() (update.InstallMethod, error) {
			return update.DetectCurrentInstallMethod()
		},
		downloadAndReplace: defaultDownloadAndReplace,
		fetchReleaseNotes: func(version string) (string, error) {
			return update.FetchReleaseNotes(client, "https://api.github.com", version)
		},
	}

	cmd := &cobra.Command{
		Use:     "self-update",
		Aliases: []string{"update"},
		Short:   "Update pb to the latest version",
		Args:    cobra.NoArgs,
		RunE:    sc.run,
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
		fmt.Fprintf(out, "pb is up to date (v%s)\n", sc.version)
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
		fmt.Fprintf(out, "Updating pb to v%s...\n", latest)
		if err := sc.downloadAndReplace(latest); err != nil {
			if errors.Is(err, os.ErrPermission) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Permission denied. Try: sudo pb self-update")
			}
			return err
		}
		fmt.Fprintf(out, "Updated pb: v%s → v%s\n", sc.version, latest)
		if notes, err := sc.fetchReleaseNotes(latest); err == nil && notes != "" {
			fmt.Fprintf(out, "\nChangelog (v%s):\n%s\n", latest, notes)
		}
	}

	return nil
}

func defaultDownloadAndReplace(version string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	// Resolve symlinks to get the real binary path
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	mainBin, agentBin, err := update.DownloadAndVerify(version, runtime.GOOS, runtime.GOARCH, "")
	if err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	defer os.RemoveAll(filepath.Dir(mainBin))

	binaryDir := filepath.Dir(realPath)

	if err := update.ReplaceBinary(mainBin, realPath); err != nil {
		return err
	}
	if err := update.RecreateSymlink(binaryDir, "plan-bender", "pb"); err != nil {
		return fmt.Errorf("recreating symlink: %w", err)
	}

	agentTargetPath := filepath.Join(binaryDir, "plan-bender-agent")
	if err := update.ReplaceBinary(agentBin, agentTargetPath); err != nil {
		return fmt.Errorf("replacing agent binary: %w", err)
	}
	if err := update.RecreateSymlink(binaryDir, "plan-bender-agent", "pba"); err != nil {
		return fmt.Errorf("recreating agent symlink: %w", err)
	}

	return nil
}
