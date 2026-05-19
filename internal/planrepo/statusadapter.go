package planrepo

import (
	"context"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
)

// prodStatusStore is the production wiring for internal/status.Store. It is
// a thin translator over planrepo.Plans — no business logic. Each Owner call
// runs inside one planrepo session that holds the plan lock from Open through
// Save (which delegates to PlanSession.Commit) until Close.
type prodStatusStore struct {
	plans *Plans
	cfg   config.Config
}

func newProdStatusStore(plansDir string, cfg config.Config) *prodStatusStore {
	return &prodStatusStore{plans: NewProd(plansDir), cfg: cfg}
}

// NewProdStatusOwner returns a status.Owner backed by a planrepo-session
// adapter rooted at plansDir. CLI commands that perform a single transition
// (retry, complete) call this directly. Long-running callers like Dispatcher
// memoize the Owner via Dispatcher.statusOwner to avoid re-allocating per Run.
func NewProdStatusOwner(plansDir string, cfg config.Config) *status.Owner {
	return status.New(newProdStatusStore(plansDir, cfg))
}

func (s *prodStatusStore) OpenSession(ctx context.Context, slug string) (status.Session, error) {
	sess, err := s.plans.OpenContext(ctx, slug)
	if err != nil {
		return nil, err
	}
	return &prodStatusSession{sess: sess, cfg: s.cfg}, nil
}

// prodStatusSession adapts one planrepo.PlanSession to status.Session. Owner
// only mutates a single issue per call, so Save stages the update and commits
// in one step before returning control.
type prodStatusSession struct {
	sess *PlanSession
	cfg  config.Config
}

func (p *prodStatusSession) Issues() []schema.Issue {
	return p.sess.Snapshot().Issues
}

func (p *prodStatusSession) Save(issue schema.Issue) error {
	if err := p.sess.UpdateIssue(issue); err != nil {
		return err
	}
	return p.sess.Commit(p.cfg)
}

func (p *prodStatusSession) Close() error {
	return p.sess.Close()
}
