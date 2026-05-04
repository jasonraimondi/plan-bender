package planrepo

import (
	"errors"
	"fmt"
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// ErrSessionClosed is returned by mutation methods, Snapshot, and Commit when
// called after Close. Use errors.Is to detect.
var ErrSessionClosed = errors.New("planrepo: session is closed")

// ErrIssueNotInSession wraps the error returned by UpdateIssue when the target
// issue ID is not in the in-session snapshot.
var ErrIssueNotInSession = errors.New("planrepo: issue not in session")

// ErrIssueIDExists wraps the error returned by CreateIssue when an issue with
// the same ID is already in the in-session snapshot.
var ErrIssueIDExists = errors.New("planrepo: issue id already exists")

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
	release     func() error
	releaseErr  error

	closed bool
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

// OpenOrCreate behaves like Open when the plan exists. When the plan
// directory is missing entirely, it returns a session with an empty in-session
// snapshot so callers can stage a fresh PRD and Commit. A plan dir that exists
// but is incomplete (missing prd.yaml or issues dir) still returns the load
// error from Open so half-written state surfaces loudly rather than silently
// being treated as fresh.
func (p *Plans) OpenOrCreate(slug string) (*PlanSession, error) {
	release, err := p.adapters.Lock(p.plansDir)
	if err != nil {
		return nil, err
	}
	if !planDirExists(p.adapters.FS, slug) {
		return &PlanSession{
			plans:             p,
			slug:              slug,
			snapshot:          &Snapshot{Slug: slug},
			baselineFilenames: map[int]string{},
			dirtyIssues:       map[int]bool{},
			release:           release,
		}, nil
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

// Snapshot returns the current in-session snapshot with a deep copy of every
// slice and pointer field, so caller mutation cannot bleed into session state.
// Mutations made via UpdatePrd, UpdateIssue, or CreateIssue are reflected on
// subsequent calls. Returns ErrSessionClosed if the session has been closed.
func (s *PlanSession) Snapshot() (Snapshot, error) {
	if s.closed {
		return Snapshot{}, ErrSessionClosed
	}
	issues := make([]schema.IssueYaml, len(s.snapshot.Issues))
	for i := range s.snapshot.Issues {
		issues[i] = cloneIssue(s.snapshot.Issues[i])
	}
	return Snapshot{
		Slug:   s.snapshot.Slug,
		PRD:    clonePrd(s.snapshot.PRD),
		Issues: issues,
	}, nil
}

func cloneIssue(iss schema.IssueYaml) schema.IssueYaml {
	iss.Labels = cloneStringSlice(iss.Labels)
	iss.BlockedBy = cloneIntSlice(iss.BlockedBy)
	iss.Blocking = cloneIntSlice(iss.Blocking)
	iss.AcceptanceCriteria = cloneStringSlice(iss.AcceptanceCriteria)
	iss.Steps = cloneStringSlice(iss.Steps)
	iss.UseCases = cloneStringSlice(iss.UseCases)
	iss.Assignee = clonePtrString(iss.Assignee)
	iss.Branch = clonePtrString(iss.Branch)
	iss.PR = clonePtrString(iss.PR)
	iss.LinearID = clonePtrString(iss.LinearID)
	iss.Notes = clonePtrString(iss.Notes)
	iss.Headed = clonePtrBool(iss.Headed)
	return iss
}

func clonePrd(prd schema.PrdYaml) schema.PrdYaml {
	prd.InScope = cloneStringSlice(prd.InScope)
	prd.OutOfScope = cloneStringSlice(prd.OutOfScope)
	prd.Decisions = cloneStringSlice(prd.Decisions)
	prd.OpenQuestions = cloneStringSlice(prd.OpenQuestions)
	prd.Risks = cloneStringSlice(prd.Risks)
	prd.Validation = cloneStringSlice(prd.Validation)
	if prd.UseCases != nil {
		ucs := make([]schema.UseCase, len(prd.UseCases))
		copy(ucs, prd.UseCases)
		prd.UseCases = ucs
	}
	prd.Notes = clonePtrString(prd.Notes)
	prd.DevCommand = clonePtrString(prd.DevCommand)
	prd.BaseURL = clonePtrString(prd.BaseURL)
	if prd.Linear != nil {
		l := *prd.Linear
		prd.Linear = &l
	}
	return prd
}

func cloneStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s...)
}

func cloneIntSlice(s []int) []int {
	if s == nil {
		return nil
	}
	return append([]int(nil), s...)
}

func clonePtrString(p *string) *string {
	if p == nil {
		return nil
	}
	s := *p
	return &s
}

func clonePtrBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	b := *p
	return &b
}

// UpdatePrd replaces the in-session PRD and marks it dirty. The change is
// not written to disk until Commit succeeds.
func (s *PlanSession) UpdatePrd(prd schema.PrdYaml) error {
	if s.closed {
		return ErrSessionClosed
	}
	s.snapshot.PRD = prd
	s.dirtyPRD = true
	return nil
}

// UpdateIssue replaces an existing issue (matched by ID) in the in-session
// snapshot and marks it dirty. Returns an error if the ID is not in the
// current snapshot — use CreateIssue for new issues.
func (s *PlanSession) UpdateIssue(issue schema.IssueYaml) error {
	if s.closed {
		return ErrSessionClosed
	}
	for i := range s.snapshot.Issues {
		if s.snapshot.Issues[i].ID == issue.ID {
			s.snapshot.Issues[i] = issue
			s.dirtyIssues[issue.ID] = true
			return nil
		}
	}
	return fmt.Errorf("update issue id #%d: %w", issue.ID, ErrIssueNotInSession)
}

// CreateIssue appends a new issue to the in-session snapshot and marks it
// dirty. Returns an error if an issue with the same ID already exists in
// the session (which would also produce a filename conflict at commit).
func (s *PlanSession) CreateIssue(issue schema.IssueYaml) error {
	if s.closed {
		return ErrSessionClosed
	}
	for _, existing := range s.snapshot.Issues {
		if existing.ID == issue.ID {
			return fmt.Errorf("create issue id #%d in plan %q: %w", issue.ID, s.slug, ErrIssueIDExists)
		}
	}
	s.snapshot.Issues = append(s.snapshot.Issues, issue)
	s.dirtyIssues[issue.ID] = true
	return nil
}

// UpsertIssue creates the issue if its ID is new in the in-session snapshot,
// or updates the existing issue if the ID already exists. Use UpdateIssue or
// CreateIssue when callers need strict create-only or update-only semantics.
func (s *PlanSession) UpsertIssue(issue schema.IssueYaml) error {
	if s.closed {
		return ErrSessionClosed
	}
	for i := range s.snapshot.Issues {
		if s.snapshot.Issues[i].ID == issue.ID {
			s.snapshot.Issues[i] = issue
			s.dirtyIssues[issue.ID] = true
			return nil
		}
	}
	s.snapshot.Issues = append(s.snapshot.Issues, issue)
	s.dirtyIssues[issue.ID] = true
	return nil
}

// Close releases the plan lock and discards any uncommitted in-session
// mutations. The first call returns the lock release error (or nil);
// subsequent calls return the same error without re-running release.
func (s *PlanSession) Close() error {
	s.releaseOnce.Do(func() {
		s.closed = true
		if s.release != nil {
			s.releaseErr = s.release()
		}
	})
	return s.releaseErr
}
