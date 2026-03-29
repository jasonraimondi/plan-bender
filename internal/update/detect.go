package update

import (
	"os"
	"strings"
)

// InstallMethod identifies how plan-bender was installed.
type InstallMethod string

const (
	InstallMethodNPM    InstallMethod = "npm"
	InstallMethodDirect InstallMethod = "direct"
)

// DetectInstallMethod determines the install method from an executable path.
// Paths containing "node_modules" indicate an npm installation.
func DetectInstallMethod(execPath string) InstallMethod {
	if strings.Contains(execPath, "node_modules") {
		return InstallMethodNPM
	}
	return InstallMethodDirect
}

// DetectCurrentInstallMethod resolves the running binary's path and detects
// the install method. Returns an error if the executable path cannot be determined.
func DetectCurrentInstallMethod() (InstallMethod, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return DetectInstallMethod(execPath), nil
}
