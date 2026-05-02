package dispatch

import (
	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// prodStatusStore is the production wiring for internal/status.Store. It is
// intentionally a thin translator over backend's flock + atomic-write
// primitives — no business logic. status.Owner takes Lock, then Load/Save
// inside the held region; Save uses an unlocked PlanStore so it doesn't
// re-acquire the same flock and deadlock.
type prodStatusStore struct {
	plansDir string
}

func newProdStatusStore(plansDir string) *prodStatusStore {
	return &prodStatusStore{plansDir: plansDir}
}

func (s *prodStatusStore) Load(slug string) ([]schema.IssueYaml, error) {
	return backend.NewProdPlanStore(s.plansDir).ReadIssues(slug)
}

func (s *prodStatusStore) Save(slug string, issue schema.IssueYaml) error {
	return backend.NewUnlockedPlanStore(s.plansDir).WriteIssue(slug, &issue)
}

func (s *prodStatusStore) Lock(_ string) (func() error, error) {
	release, err := backend.LockPlanDir(s.plansDir)
	if err != nil {
		return nil, err
	}
	return func() error { release(); return nil }, nil
}
