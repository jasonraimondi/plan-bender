package planrepo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkdirAll(t *testing.T, dir string) error {
	t.Helper()
	return os.MkdirAll(dir, 0o755)
}

func writeFile(t *testing.T, path, body string) error {
	t.Helper()
	return os.WriteFile(path, []byte(body), 0o644)
}

func TestNew_UsesInjectedLockAndSurfacesErrors(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.json": issueJSON(1, "a"),
	})

	wantErr := errors.New("lock denied")
	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Write: func(_ string, _ []byte, _ fs.FileMode) error { return nil },
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Lock: func(_ string) (func(), error) {
			return nil, wantErr
		},
	}
	repo := New(plansDir, adapters)

	_, err := repo.Open("p")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestNew_LockReleasedExactlyOnceOnClose(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.json": issueJSON(1, "a"),
	})

	var releases int
	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Write: func(_ string, _ []byte, _ fs.FileMode) error { return nil },
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Lock: func(_ string) (func(), error) {
			return func() { releases++ }, nil
		},
	}
	repo := New(plansDir, adapters)

	sess, err := repo.Open("p")
	require.NoError(t, err)
	require.NoError(t, sess.Close())
	require.NoError(t, sess.Close())
	assert.Equal(t, 1, releases, "release must run exactly once across multiple Close calls")
}

func TestNew_LockReleasedWhenSnapshotLoadFails(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	// No plan written: snapshot load will fail after lock is taken.

	var releases int
	adapters := Adapters{
		FS:    os.DirFS(plansDir),
		Write: func(_ string, _ []byte, _ fs.FileMode) error { return nil },
		Mkdir: func(_ string, _ fs.FileMode) error { return nil },
		Lock: func(_ string) (func(), error) {
			return func() { releases++ }, nil
		},
	}
	repo := New(plansDir, adapters)

	_, err := repo.Open("missing")
	require.Error(t, err)
	assert.Equal(t, 1, releases, "lock must be released when load fails")
}

func TestNewProd_HasAllProductionAdaptersWired(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writePlan(t, plansDir, "p", validPrd, map[string]string{
		"1-a.json": issueJSON(1, "a"),
	})

	repo := NewProd(plansDir)
	require.NotNil(t, repo)

	sess, err := repo.Open("p")
	require.NoError(t, err)
	require.NotNil(t, sess.Snapshot())
	require.NoError(t, sess.Close())
}
