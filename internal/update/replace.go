package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReplaceBinary atomically replaces the binary at targetPath with the contents
// of newBinaryPath. It writes to a temp file in the same directory as targetPath,
// then renames (atomic on same filesystem). The result has mode 0755.
func ReplaceBinary(newBinaryPath, targetPath string) error {
	src, err := os.Open(newBinaryPath)
	if err != nil {
		return fmt.Errorf("opening new binary: %w", err)
	}
	defer src.Close()

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".plan-bender-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}

// RecreateSymlink creates (or recreates) a symlink named linkName in binaryDir
// pointing to target. Any existing file or symlink at the path is removed first.
func RecreateSymlink(binaryDir, target, linkName string) error {
	symlinkPath := filepath.Join(binaryDir, linkName)

	// Remove whatever is there (symlink, file, or nothing)
	if err := os.Remove(symlinkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing old symlink: %w", err)
	}

	if err := os.Symlink(target, symlinkPath); err != nil {
		return fmt.Errorf("creating symlink: %w", err)
	}

	return nil
}
