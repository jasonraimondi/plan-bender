//go:build !windows

package planrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LockPlanDir takes an exclusive POSIX advisory lock (flock) on a sentinel
// file inside plansDir. The returned release closes the file and drops the
// lock.
//
// The kernel resolves symlinks to the same inode, so sub-agents reaching
// plansDir through a symlink from a worktree contend on the same lock.
//
// Acquisition is a non-blocking flock poll loop: an uncontended lock is taken
// on the first attempt, and a wait for a contended lock unwinds when ctx is
// canceled or its deadline expires.
func LockPlanDir(ctx context.Context, plansDir string) (func(), error) {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plans dir for lock: %w", err)
	}
	lockPath := filepath.Join(plansDir, ".pb-lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("acquiring lock: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("acquiring lock: %w", ctx.Err())
		case <-time.After(lockPollInterval):
		}
	}
}
