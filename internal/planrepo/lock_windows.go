//go:build windows

package planrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// LockPlanDir takes an exclusive lock on a sentinel file inside plansDir
// using the Win32 LockFileEx API. The returned release unlocks and closes
// the file. Mirrors the unix flock implementation, including the non-blocking
// poll loop that unwinds when ctx is canceled or its deadline expires.
func LockPlanDir(ctx context.Context, plansDir string) (func(), error) {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plans dir for lock: %w", err)
	}
	lockPath := filepath.Join(plansDir, ".pb-lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	ol := new(windows.Overlapped)
	const flags = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	for {
		err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
				_ = f.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
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
