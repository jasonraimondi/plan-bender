package backend

import (
	"io/fs"
	"os"
	"path/filepath"
)

// prodFS returns an os.DirFS rooted at dir.
func prodFS(dir string) fs.FS {
	return os.DirFS(dir)
}

// AtomicWrite is the production WriteFunc using atomic temp+rename.
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

// prodMkdir is the production MkdirFunc.
func prodMkdir(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

// readDirSafe reads a directory, returning empty list for nonexistent dirs.
func readDirSafe(fsys fs.FS, dir string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return entries, nil
}
