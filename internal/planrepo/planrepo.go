// Package planrepo is the session-oriented boundary for plan persistence.
//
// A Plans repository owns plansDir and exposes plan sessions and plan
// listing through one file contract path. Each PlanSession holds the plan
// lock for its lifetime and exposes an immutable Snapshot of the on-disk
// PRD and issues at open time.
//
// Read and write I/O is provided through Adapters so callers can substitute
// in-memory or failing adapters in tests. NewProd wires adapters that match
// the existing on-disk contract: os filesystem reads, atomic temp+rename
// writes, recursive mkdir, and a process-shared flock on the plans dir.
package planrepo

import (
	"fmt"
	"io/fs"
)

// WriteFunc writes data to an absolute path on disk.
type WriteFunc func(path string, data []byte, perm fs.FileMode) error

// MkdirFunc creates a directory tree.
type MkdirFunc func(path string, perm fs.FileMode) error

// RemoveFunc removes the file at path. A non-existent path is not an error.
type RemoveFunc func(path string) error

// LockFunc acquires the plan lock for plansDir and returns a release closure.
// Callers invoke release exactly once when the session ends. Release errors
// (e.g. unlock or close failures) are returned so Close can surface them.
type LockFunc func(plansDir string) (release func() error, err error)

// Adapters bundles the I/O dependencies a Plans repository needs.
//
// FS and Lock are read by Open/Snapshot/Close/List. Write, Mkdir, and Remove
// are the I/O entry points used by PlanSession.Commit during the staged-write
// phase: Mkdir ensures the plan and issues directories exist before any
// write runs, Write applies each staged file atomically (in production, via
// AtomicWrite — see adapters.go), and Remove handles slug-rename cleanup and
// commit rollback. Tests substitute failing variants here to drive the commit
// rollback path.
type Adapters struct {
	FS     fs.FS
	Write  WriteFunc
	Mkdir  MkdirFunc
	Lock   LockFunc
	Remove RemoveFunc
}

// Plans is the top-level repository handle.
type Plans struct {
	plansDir string
	adapters Adapters
}

// New constructs a Plans with explicit adapters. plansDir is the absolute
// directory passed to the lock and write adapters; adapters.FS is rooted at
// plansDir and used for reads. Panics if any required adapter is nil so
// misconfigured callers fail loudly at construction rather than at first use.
func New(plansDir string, adapters Adapters) *Plans {
	if err := validateAdapters(adapters); err != nil {
		panic(fmt.Errorf("planrepo.New: %w", err))
	}
	return &Plans{plansDir: plansDir, adapters: adapters}
}

func validateAdapters(a Adapters) error {
	switch {
	case a.FS == nil:
		return fmt.Errorf("nil adapter: FS")
	case a.Write == nil:
		return fmt.Errorf("nil adapter: Write")
	case a.Mkdir == nil:
		return fmt.Errorf("nil adapter: Mkdir")
	case a.Lock == nil:
		return fmt.Errorf("nil adapter: Lock")
	case a.Remove == nil:
		return fmt.Errorf("nil adapter: Remove")
	}
	return nil
}

// NewProd wires production adapters: an os filesystem reader rooted at
// plansDir, atomic temp+rename writes, recursive mkdir, os.Remove, and a
// flock on .pb-lock inside plansDir that serializes write-side access across
// processes sharing plansDir.
func NewProd(plansDir string) *Plans {
	return New(plansDir, Adapters{
		FS:     prodFS(plansDir),
		Write:  AtomicWrite,
		Mkdir:  prodMkdir,
		Lock:   LockPlanDir,
		Remove: prodRemove,
	})
}
