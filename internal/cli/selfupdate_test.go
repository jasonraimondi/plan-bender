package cli

import (
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
	assert.Contains(t, out.String(), "plan-bender is up to date (v1.2.3)")
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

func TestSelfUpdate_DirectBinary(t *testing.T) {
	cmd := NewSelfUpdateCmd("1.0.0")
	sc := selfUpdateFromCmd(cmd)
	sc.checkForUpdate = func(currentVersion string) (string, bool, error) {
		return "1.2.3", true, nil
	}
	sc.detectInstallMethod = func() (update.InstallMethod, error) {
		return update.InstallMethodDirect, nil
	}

	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())
	output := out.String()
	assert.Contains(t, output, "Updating plan-bender")
	assert.Contains(t, output, "1.2.3")
}
