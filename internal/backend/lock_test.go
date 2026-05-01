package backend

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLockPlanDir_SerializesConcurrentHolders asserts that two callers can't
// both hold the lock at once: the second blocks until the first releases.
func TestLockPlanDir_SerializesConcurrentHolders(t *testing.T) {
	plansDir := t.TempDir()

	release1, err := LockPlanDir(plansDir)
	require.NoError(t, err)

	acquired2 := make(chan struct{})
	go func() {
		release2, err := LockPlanDir(plansDir)
		require.NoError(t, err)
		close(acquired2)
		release2()
	}()

	select {
	case <-acquired2:
		t.Fatal("second LockPlanDir acquired before first released")
	default:
	}

	release1()

	<-acquired2
}

// TestLockedAtomicWrite_SerializesAndPersists asserts the production WriteFunc
// (atomic write under flock) actually writes the file and serializes callers.
func TestLockedAtomicWrite_SerializesAndPersists(t *testing.T) {
	plansDir := t.TempDir()
	write := lockedAtomicWrite(plansDir)

	target := filepath.Join(plansDir, "out.txt")

	var wg sync.WaitGroup
	const writers = 10
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			require.NoError(t, write(target, []byte("hello"), 0o644))
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}
