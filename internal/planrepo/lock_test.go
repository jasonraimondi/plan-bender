package planrepo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLockPlanDir_UncontendedAcquiresImmediately asserts the uncontended path
// behaves as before: an unheld lock is taken without consulting ctx.
func TestLockPlanDir_UncontendedAcquiresImmediately(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plans")

	release, err := LockPlanDir(context.Background(), dir)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

// TestLockPlanDir_CanceledContextInterruptsHeldLock proves a wait for a
// contended plan lock unwinds when the waiter's context is canceled, instead
// of blocking forever on the holder.
func TestLockPlanDir_CanceledContextInterruptsHeldLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plans")

	release, err := LockPlanDir(context.Background(), dir)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		rel, err := LockPlanDir(ctx, dir)
		if rel != nil {
			rel()
		}
		result <- err
	}()

	// The waiter must still be blocked on the held lock before we cancel.
	select {
	case err := <-result:
		t.Fatalf("second LockPlanDir returned before cancellation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("canceled context did not interrupt the wait for a held lock")
	}
}

// TestLockPlanDir_DeadlineInterruptsHeldLock proves a timed-out context also
// unwinds a wait for a contended lock.
func TestLockPlanDir_DeadlineInterruptsHeldLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plans")

	release, err := LockPlanDir(context.Background(), dir)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	rel, err := LockPlanDir(ctx, dir)
	if rel != nil {
		rel()
	}
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), 2*time.Second, "wait should end at the deadline, not hang")
}
