package cli

import (
	"os"
	"testing"
)

// TestMain isolates HOME and backend-related env vars so that a developer's
// local ~/.config/plan-bender/defaults.yaml and $LINEAR_* exports do not leak
// into tests that transitively call config.Load via setup/doctor/generate.
func TestMain(m *testing.M) {
	tmpHome, err := os.MkdirTemp("", "pb-testhome-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", tmpHome)
	os.Unsetenv("LINEAR_API_KEY")
	os.Unsetenv("LINEAR_TEAM")
	os.Unsetenv("LINEAR_TEAM_ID")

	code := m.Run()

	os.RemoveAll(tmpHome)
	os.Exit(code)
}
