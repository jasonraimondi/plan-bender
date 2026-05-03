package planrepo

import (
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Snapshot is an immutable in-session view of a plan at Open time.
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
	snap, err := loadSnapshot(p.adapters.FS, slug)
	if err != nil {
		release()
		return nil, err
	}
	return &PlanSession{
		plans:    p,
		slug:     slug,
		snapshot: snap,
		release:  release,
	}, nil
}

// Snapshot returns the in-session snapshot. The caller must treat the
// returned value as read-only; mutation is not part of this API.
func (s *PlanSession) Snapshot() *Snapshot {
	return s.snapshot
}

// Close releases the plan lock. Idempotent: subsequent calls are no-ops.
func (s *PlanSession) Close() error {
	s.releaseOnce.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
	return nil
}
