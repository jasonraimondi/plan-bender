package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceBinary(t *testing.T) {
	t.Run("replaces target with new binary content", func(t *testing.T) {
		dir := t.TempDir()

		// Create "old" binary
		targetPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(targetPath, []byte("old-binary"), 0o755))

		// Create "new" binary in a separate temp dir
		srcDir := t.TempDir()
		newBinaryPath := filepath.Join(srcDir, "plan-bender")
		require.NoError(t, os.WriteFile(newBinaryPath, []byte("new-binary-v2"), 0o755))

		err := ReplaceBinary(newBinaryPath, targetPath)
		require.NoError(t, err)

		data, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, "new-binary-v2", string(data))

		info, err := os.Stat(targetPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("returns error when source does not exist", func(t *testing.T) {
		dir := t.TempDir()
		targetPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(targetPath, []byte("old"), 0o755))

		err := ReplaceBinary("/nonexistent/path/binary", targetPath)
		require.Error(t, err)
	})

	t.Run("returns permission error for read-only target directory", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Getuid() == 0 {
			t.Skip("permission test not reliable on windows or as root")
		}

		dir := t.TempDir()
		targetPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(targetPath, []byte("old"), 0o755))

		srcDir := t.TempDir()
		newBinaryPath := filepath.Join(srcDir, "plan-bender")
		require.NoError(t, os.WriteFile(newBinaryPath, []byte("new"), 0o755))

		// Make target directory read-only so temp file creation fails
		require.NoError(t, os.Chmod(dir, 0o555))
		t.Cleanup(func() { os.Chmod(dir, 0o755) })

		err := ReplaceBinary(newBinaryPath, targetPath)
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrPermission)
	})
}

func TestRecreateSymlink(t *testing.T) {
	t.Run("creates pb symlink pointing to plan-bender", func(t *testing.T) {
		dir := t.TempDir()
		binaryPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(binaryPath, []byte("binary"), 0o755))

		err := RecreateSymlink(dir, "plan-bender", "pb")
		require.NoError(t, err)

		symlinkPath := filepath.Join(dir, "pb")
		target, err := os.Readlink(symlinkPath)
		require.NoError(t, err)
		assert.Equal(t, "plan-bender", target)
	})

	t.Run("creates pba symlink pointing to plan-bender-agent", func(t *testing.T) {
		dir := t.TempDir()
		binaryPath := filepath.Join(dir, "plan-bender-agent")
		require.NoError(t, os.WriteFile(binaryPath, []byte("agent-binary"), 0o755))

		err := RecreateSymlink(dir, "plan-bender-agent", "pba")
		require.NoError(t, err)

		symlinkPath := filepath.Join(dir, "pba")
		target, err := os.Readlink(symlinkPath)
		require.NoError(t, err)
		assert.Equal(t, "plan-bender-agent", target)
	})

	t.Run("replaces existing symlink", func(t *testing.T) {
		dir := t.TempDir()
		binaryPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(binaryPath, []byte("binary"), 0o755))

		// Create existing symlink pointing elsewhere
		symlinkPath := filepath.Join(dir, "pb")
		require.NoError(t, os.Symlink("old-target", symlinkPath))

		err := RecreateSymlink(dir, "plan-bender", "pb")
		require.NoError(t, err)

		target, err := os.Readlink(symlinkPath)
		require.NoError(t, err)
		assert.Equal(t, "plan-bender", target)
	})

	t.Run("replaces existing regular file", func(t *testing.T) {
		dir := t.TempDir()
		binaryPath := filepath.Join(dir, "plan-bender")
		require.NoError(t, os.WriteFile(binaryPath, []byte("binary"), 0o755))

		// Create existing regular file named pb
		symlinkPath := filepath.Join(dir, "pb")
		require.NoError(t, os.WriteFile(symlinkPath, []byte("not-a-symlink"), 0o755))

		err := RecreateSymlink(dir, "plan-bender", "pb")
		require.NoError(t, err)

		target, err := os.Readlink(symlinkPath)
		require.NoError(t, err)
		assert.Equal(t, "plan-bender", target)
	})
}
