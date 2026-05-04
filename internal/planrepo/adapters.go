package planrepo

import (
	"io/fs"
	"os"
	"path/filepath"
)

// prodFS returns an os.DirFS rooted at dir.
func prodFS(dir string) fs.FS {
	return os.DirFS(dir)
}

// AtomicWrite writes data to path via temp file + rename so readers never
// observe a partial file. The temp file is created in the destination
// directory so the rename stays on the same filesystem.
func AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pb-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// prodMkdir creates a directory tree.
func prodMkdir(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}
