//go:build windows

package dispatch

import "os/exec"

// configureProcessGroup is a no-op on Windows, which has no POSIX process
// groups. exec.CommandContext's default Cancel plus cmd.WaitDelay still bound
// the subprocess lifetime.
func configureProcessGroup(cmd *exec.Cmd) {}
