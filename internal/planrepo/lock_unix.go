//go:build !windows

package planrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockPlanDir takes an exclusive POSIX advisory lock (flock) on a sentinel
// file inside plansDir. The returned release closes the file and drops the
// lock.
//
// The kernel resolves symlinks to the same inode, so sub-agents reaching
// plansDir through a symlink from a worktree contend on the same lock.
func LockPlanDir(plansDir string) (func(), error) {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plans dir for lock: %w", err)
	}
	lockPath := filepath.Join(plansDir, ".pb-lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
