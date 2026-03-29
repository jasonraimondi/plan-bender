package cli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/update"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfUpdate_DevVersion(t *testing.T) {
	cmd := NewSelfUpdateCmd("dev")
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "Skipping update check for development build")
}

func TestSelfUpdate_AlreadyLatest(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.2.3")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", false, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodDirect, nil
	}

	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "pb is up to date (v1.2.3)")
}

func TestSelfUpdate_NPMDetected(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.0.0")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", true, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodNPM, nil
	}

	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	output := out.String()
	assert.Contains(t, output, "A newer version is available: 1.0.0 → 1.2.3")
	assert.Contains(t, output, "Run: npm install -g @jasonraimondi/plan-bender@latest")
}

func TestSelfUpdate_DirectBinary_DownloadsAndReplaces(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.0.0")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", true, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodDirect, nil
	}

	var downloadCalled bool
	sc.downloadAndReplace = func(version string) error {
		assert.Equal(t, "1.2.3", version)
		downloadCalled = true
		return nil
	}

	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.True(t, downloadCalled)
	output := out.String()
	assert.Contains(t, output, "Updating pb to v1.2.3...")
	assert.Contains(t, output, "Updated pb: v1.0.0 → v1.2.3")
}

func TestSelfUpdate_DirectBinary_PermissionDenied(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.0.0")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", true, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodDirect, nil
	}
	sc.downloadAndReplace = func(version string) error {
		return os.ErrPermission
	}

	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrPermission)
	assert.Contains(t, errOut.String(), "Permission denied. Try: sudo pb self-update")
}

func TestSelfUpdate_DirectBinary_DownloadError(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.0.0")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", true, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodDirect, nil
	}
	sc.downloadAndReplace = func(version string) error {
		return errors.New("network failure")
	}

	var out strings.Builder
	cmd.SetOut(&out)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network failure")
}
