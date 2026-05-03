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

import "io/fs"

// WriteFunc writes data to an absolute path on disk.
type WriteFunc func(path string, data []byte, perm fs.FileMode) error

// MkdirFunc creates a directory tree.
type MkdirFunc func(path string, perm fs.FileMode) error

// LockFunc acquires the plan lock for plansDir and returns a release closure.
// Callers invoke release exactly once when the session ends. Release errors
// (e.g. unlock or close failures) are returned so Close can surface them.
type LockFunc func(plansDir string) (release func() error, err error)

// Adapters bundles the I/O dependencies a Plans repository needs.
//
// FS and Lock are read by Open/Snapshot/Close/List. Write and Mkdir are the
// I/O entry points used by PlanSession.Commit during the staged-write phase:
// Mkdir ensures the plan and issues directories exist before any write runs,
// and Write applies each staged file atomically (in production, via
// AtomicWrite — see adapters.go). Tests substitute failing variants here to
// drive the commit rollback path.
type Adapters struct {
	FS    fs.FS
	Write WriteFunc
	Mkdir MkdirFunc
	Lock  LockFunc
}

// Plans is the top-level repository handle.
type Plans struct {
	plansDir string
	adapters Adapters
}

// New constructs a Plans with explicit adapters. plansDir is the absolute
// directory passed to the lock and write adapters; adapters.FS is rooted at
// plansDir and used for reads.
func New(plansDir string, adapters Adapters) *Plans {
	return &Plans{plansDir: plansDir, adapters: adapters}
}

// NewProd wires production adapters: an os filesystem reader rooted at
// plansDir, atomic temp+rename writes, recursive mkdir, and a flock on
// .pb-lock inside plansDir that serializes write-side access across
// processes sharing plansDir.
func NewProd(plansDir string) *Plans {
	return New(plansDir, Adapters{
		FS:    prodFS(plansDir),
		Write: AtomicWrite,
		Mkdir: prodMkdir,
		Lock:  LockPlanDir,
	})
}
