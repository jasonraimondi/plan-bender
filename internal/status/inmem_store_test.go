package status

import (
	"fmt"
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// inMemStore is the in-memory Store fake used by Owner tests. Per-slug Lock
// gives the same serialization guarantee as the production flock-based
// store, so concurrency tests are meaningful here. Production callers use
// the planrepo-backed adapter — this fake is package-test only.
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

// Load returns a defensive copy of the seeded issues for slug. Used by tests
// that want to read state out-of-band without going through a session.
func (s *inMemStore) Load(slug string) ([]schema.IssueYaml, error) {
	list, ok := s.issues[slug]
	if !ok {
		return nil, fmt.Errorf("plan %q not seeded", slug)
	}
	cp := make([]schema.IssueYaml, len(list))
	copy(cp, list)
	return cp, nil
}

func (s *inMemStore) OpenSession(slug string) (Session, error) {
	s.mu.Lock()
	m, ok := s.locks[slug]
	if !ok {
		m = &sync.Mutex{}
		s.locks[slug] = m
	}
	s.mu.Unlock()
	m.Lock()

	if _, ok := s.issues[slug]; !ok {
		m.Unlock()
		return nil, fmt.Errorf("plan %q not seeded", slug)
	}
	return &inMemSession{store: s, slug: slug, mu: m}, nil
}

type inMemSession struct {
	store *inMemStore
	slug  string
	mu    *sync.Mutex
	once  sync.Once
}

func (s *inMemSession) Issues() []schema.IssueYaml {
	list := s.store.issues[s.slug]
	cp := make([]schema.IssueYaml, len(list))
	copy(cp, list)
	return cp
}

func (s *inMemSession) Save(issue schema.IssueYaml) error {
	list := s.store.issues[s.slug]
	for i := range list {
		if list[i].ID == issue.ID {
			list[i] = issue
			s.store.saves++
			return nil
		}
	}
	return fmt.Errorf("issue #%d not in plan %q", issue.ID, s.slug)
}

func (s *inMemSession) Close() error {
	s.once.Do(func() { s.mu.Unlock() })
	return nil
}
