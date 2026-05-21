//go:build !windows

package planrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTryFlock_CrashReleasesLock proves the safety property dispatch's per-slug
// lock relies on: when the holding process dies WITHOUT calling release(), the
// kernel drops the flock on FD close, so a crashed `pba dispatch` doesn't leave
// the slug permanently locked. The test re-execs its own binary to hold the lock
// in a child, confirms the parent is locked out while the child lives, SIGKILLs
// the child, and asserts the parent can then re-acquire.
func TestTryFlock_CrashReleasesLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "x.lock")
	readyPath := filepath.Join(dir, "ready")

	cmd := exec.Command(os.Args[0], "-test.run=TestFlockHelperProcess")
	cmd.Env = append(os.Environ(),
		"PB_FLOCK_HELPER=1",
		"PB_FLOCK_PATH="+lockPath,
		"PB_FLOCK_READY="+readyPath,
	)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Wait until the child actually holds the lock (signalled by the ready file).
	require.Eventually(t, func() bool {
		_, err := os.Stat(readyPath)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "child never acquired the lock")

	// While the child lives, the parent must be locked out — proves the child
	// holds a real exclusive lock, so the post-kill reacquire is meaningful.
	rel, err := TryFlock(lockPath)
	require.ErrorIs(t, err, ErrLocked)
	require.Nil(t, rel)

	// Crash the holder: kill it without ever running release().
	require.NoError(t, cmd.Process.Kill())
	_, _ = cmd.Process.Wait()

	// The kernel must release the lock as the dead process's FDs close. Allow a
	// brief window for teardown; it should succeed near-instantly.
	require.Eventually(t, func() bool {
		r, e := TryFlock(lockPath)
		if e == nil {
			r()
			return true
		}
		return false
	}, 3*time.Second, 10*time.Millisecond, "kernel must release flock after the holder is killed")
}

// TestFlockHelperProcess is the child half of TestTryFlock_CrashReleasesLock.
// It runs as a normal no-op unless re-exec'd with PB_FLOCK_HELPER=1, in which
// case it takes the lock, signals readiness, and blocks forever (never
// releasing) so the parent can kill it mid-hold.
func TestFlockHelperProcess(t *testing.T) {
	if os.Getenv("PB_FLOCK_HELPER") != "1" {
		return
	}
	rel, err := TryFlock(os.Getenv("PB_FLOCK_PATH"))
	if err != nil {
		os.Exit(2)
	}
	_ = rel // intentionally never released; the parent SIGKILLs us
	if err := os.WriteFile(os.Getenv("PB_FLOCK_READY"), []byte("1"), 0o644); err != nil {
		os.Exit(3)
	}
	// Block on a timer rather than `select {}`: a bare select with no other
	// runnable goroutines trips Go's all-goroutines-asleep deadlock detector,
	// which would crash (and so release the lock) before the parent's check.
	time.Sleep(time.Hour)
}
