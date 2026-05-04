//go:build windows

package planrepo

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// LockPlanDir takes an exclusive lock on a sentinel file inside plansDir
// using the Win32 LockFileEx API. The returned release unlocks and closes
// the file. Mirrors the unix flock implementation.
func LockPlanDir(plansDir string) (func(), error) {
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating plans dir for lock: %w", err)
	}
	lockPath := filepath.Join(plansDir, ".pb-lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
		_ = f.Close()
	}, nil
}
