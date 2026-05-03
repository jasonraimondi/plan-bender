package planrepo

import (
	"fmt"
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Snapshot is an in-session view of a plan. Open populates it from disk and
// the session mutation methods update it in place; the file API treats it as
// the single source of truth between Open and Commit/Close.
type Snapshot struct {
	Slug   string
	PRD    schema.PrdYaml
	Issues []schema.IssueYaml
}

// PlanSession is one open session against a single plan. It holds the plan
// lock for its lifetime; Close releases the lock exactly once.
type PlanSession struct {
	plans    *Plans
	slug     string
	snapshot *Snapshot

	// baselineFilenames is the on-disk filename for each issue ID at Open
	// time. Commit compares the current canonical {id}-{slug}.yaml against
	// this map to detect slug renames.
	baselineFilenames map[int]string

	// dirtyPRD is true once UpdatePrd is called.
	dirtyPRD bool

	// dirtyIssues holds the IDs of issues that have been mutated or created
	// in-session. Commit only writes files for IDs in this set.
	dirtyIssues map[int]bool

	releaseOnce sync.Once
	release     func()
}

// Open acquires the plan lock and loads a snapshot for slug. If lock
// acquisition or snapshot loading fails, no lock is held on return.
func (p *Plans) Open(slug string) (*PlanSession, error) {
	release, err := p.adapters.Lock(p.plansDir)
	if err != nil {
		return nil, err
	}
	snap, filenames, err := loadSnapshotWithFilenames(p.adapters.FS, slug)
	if err != nil {
		release()
		return nil, err
	}
	return &PlanSession{
		plans:             p,
		slug:              slug,
		snapshot:          snap,
		baselineFilenames: filenames,
		dirtyIssues:       map[int]bool{},
		release:           release,
	}, nil
}

// Snapshot returns the current in-session snapshot. Mutations made via
// UpdatePrd, UpdateIssue, or CreateIssue are reflected on subsequent calls.
// Callers must treat the returned value as read-only; mutate through the
// session methods instead.
func (s *PlanSession) Snapshot() *Snapshot {
	return s.snapshot
}

// UpdatePrd replaces the in-session PRD and marks it dirty. The change is
// not written to disk until Commit succeeds.
func (s *PlanSession) UpdatePrd(prd schema.PrdYaml) error {
	s.snapshot.PRD = prd
	s.dirtyPRD = true
	return nil
}

// UpdateIssue replaces an existing issue (matched by ID) in the in-session
// snapshot and marks it dirty. Returns an error if the ID is not in the
// current snapshot — use CreateIssue for new issues.
func (s *PlanSession) UpdateIssue(issue schema.IssueYaml) error {
	for i := range s.snapshot.Issues {
		if s.snapshot.Issues[i].ID == issue.ID {
			s.snapshot.Issues[i] = issue
			s.dirtyIssues[issue.ID] = true
			return nil
		}
	}
	return fmt.Errorf("update issue: id #%d not in session snapshot", issue.ID)
}

// CreateIssue appends a new issue to the in-session snapshot and marks it
// dirty. Returns an error if an issue with the same ID already exists in
// the session (which would also produce a filename conflict at commit).
func (s *PlanSession) CreateIssue(issue schema.IssueYaml) error {
	for _, existing := range s.snapshot.Issues {
		if existing.ID == issue.ID {
			return fmt.Errorf("create issue: id #%d already exists in plan %q", issue.ID, s.slug)
		}
	}
	s.snapshot.Issues = append(s.snapshot.Issues, issue)
	s.dirtyIssues[issue.ID] = true
	return nil
}

// Close releases the plan lock and discards any uncommitted in-session
// mutations. Idempotent: subsequent calls are no-ops.
func (s *PlanSession) Close() error {
	s.releaseOnce.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
	return nil
}
