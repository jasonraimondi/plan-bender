package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

func writeAdapterIssue(t *testing.T, plansDir, slug string, iss schema.IssueYaml) {
	t.Helper()
	dir := filepath.Join(plansDir, slug, "issues")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	data, err := yaml.Marshal(iss)
	require.NoError(t, err)
	path := filepath.Join(dir, fmt.Sprintf("%d-%s.yaml", iss.ID, iss.Slug))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func TestProdStatusStore_LoadSaveRoundTrip(t *testing.T) {
	plansDir := t.TempDir()
	iss := schema.IssueYaml{ID: 7, Slug: "alpha", Name: "alpha", Status: "todo"}
	writeAdapterIssue(t, plansDir, "demo", iss)

	store := newProdStatusStore(plansDir)

	got, err := store.Load("demo")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "todo", got[0].Status)

	release, err := store.Lock("demo")
	require.NoError(t, err)
	got[0].Status = "in-progress"
	require.NoError(t, store.Save("demo", got[0]))
	require.NoError(t, release())

	reloaded, err := store.Load("demo")
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	assert.Equal(t, "in-progress", reloaded[0].Status)
}

func TestProdStatusStore_LoadMissingPlanReturnsError(t *testing.T) {
	plansDir := t.TempDir()
	store := newProdStatusStore(plansDir)

	_, err := store.Load("nope")
	require.Error(t, err)
}

// TestProdStatusStore_LockSerializesConcurrent asserts the per-process flock
// serializes Lock callers — the second waits until the first releases.
func TestProdStatusStore_LockSerializesConcurrent(t *testing.T) {
	plansDir := t.TempDir()
	store := newProdStatusStore(plansDir)

	release1, err := store.Lock("demo")
	require.NoError(t, err)

	acquired2 := make(chan struct{})
	var release2 func() error
	go func() {
		var err error
		release2, err = store.Lock("demo")
		require.NoError(t, err)
		close(acquired2)
	}()

	select {
	case <-acquired2:
		t.Fatal("second Lock acquired before first released")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, release1())

	select {
	case <-acquired2:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock never acquired after first released")
	}
	require.NoError(t, release2())
}

// TestProdStatusStore_ConcurrentSaveSerializes asserts that multiple goroutines
// taking Lock + Save individually don't corrupt the file or interleave writes.
func TestProdStatusStore_ConcurrentSaveSerializes(t *testing.T) {
	plansDir := t.TempDir()
	iss := schema.IssueYaml{ID: 1, Slug: "x", Name: "x", Status: "todo"}
	writeAdapterIssue(t, plansDir, "p", iss)

	store := newProdStatusStore(plansDir)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := store.Lock("p")
			require.NoError(t, err)
			defer func() { require.NoError(t, release()) }()

			issues, err := store.Load("p")
			require.NoError(t, err)
			require.Len(t, issues, 1)
			issues[0].Status = "in-progress"
			require.NoError(t, store.Save("p", issues[0]))
		}()
	}
	wg.Wait()

	got, err := store.Load("p")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "in-progress", got[0].Status)
}
