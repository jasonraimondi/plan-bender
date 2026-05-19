//go:build !windows

package dispatch

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup places cmd in its own process group and replaces the
// ctx-cancel kill so the whole group is signaled, not just the direct child.
//
// exec.CommandContext's default Cancel sends SIGKILL to the direct child only.
// A surviving grandchild (an MCP server, a bash-tool subprocess) that inherited
// the stdout/stderr pipe keeps the write end open, so the I/O-copy goroutine and
// cmd.Wait() never return. Signaling the negative PID — the process group —
// reaps grandchildren too, letting the pipes close naturally.
func configureProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Setpgid with the default Pgid 0 makes the child its own group leader,
		// so its PID is the PGID; negating it signals the entire group.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
