package backend

import (
	"io/fs"
)

// lockedAtomicWrite returns a WriteFunc that takes the plansDir lock for the
// duration of each write, then delegates to AtomicWrite.
func lockedAtomicWrite(plansDir string) WriteFunc {
	return func(path string, data []byte, perm fs.FileMode) error {
		release, err := LockPlanDir(plansDir)
		if err != nil {
			return err
		}
		defer release()
		return AtomicWrite(path, data, perm)
	}
}
