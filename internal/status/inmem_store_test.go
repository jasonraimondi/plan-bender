package status

import (
	"fmt"
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// inMemStore is the in-memory Store fake used by Owner tests. Per-slug Lock
// gives the same serialization guarantee as the production flock-based
// store, so concurrency tests are meaningful here. Production callers use
// the adapter introduced in issue #2 — this fake is package-test only.
type inMemStore struct {
	mu     sync.Mutex
	locks  map[string]*sync.Mutex
	issues map[string][]schema.IssueYaml
	saves  int
}

func newInMemStore() *inMemStore {
	return &inMemStore{
		locks:  make(map[string]*sync.Mutex),
		issues: make(map[string][]schema.IssueYaml),
	}
}

func (s *inMemStore) seed(slug string, issues ...schema.IssueYaml) {
	cp := make([]schema.IssueYaml, len(issues))
	copy(cp, issues)
	s.issues[slug] = cp
}

func (s *inMemStore) Load(slug string) ([]schema.IssueYaml, error) {
	list, ok := s.issues[slug]
	if !ok {
		return nil, fmt.Errorf("plan %q not seeded", slug)
	}
	cp := make([]schema.IssueYaml, len(list))
	copy(cp, list)
	return cp, nil
}

func (s *inMemStore) Save(slug string, issue schema.IssueYaml) error {
	list, ok := s.issues[slug]
	if !ok {
		return fmt.Errorf("plan %q not seeded", slug)
	}
	for i := range list {
		if list[i].ID == issue.ID {
			list[i] = issue
			s.saves++
			return nil
		}
	}
	return fmt.Errorf("issue #%d not in plan %q", issue.ID, slug)
}

func (s *inMemStore) Lock(slug string) (func() error, error) {
	s.mu.Lock()
	m, ok := s.locks[slug]
	if !ok {
		m = &sync.Mutex{}
		s.locks[slug] = m
	}
	s.mu.Unlock()
	m.Lock()
	return func() error { m.Unlock(); return nil }, nil
}
