package status

import "github.com/jasonraimondi/plan-bender/internal/schema"

// Store opens one Session per Owner read-modify-commit cycle. The session
// shape lets the production adapter back Owner with a single plan-repository
// session that spans Open → mutation → Commit, instead of three independent
// Lock/Load/Save calls that would each re-acquire the plan lock.
type Store interface {
	OpenSession(slug string) (Session, error)
}

// Session is one open lock-holding view onto a plan. It must be Closed once;
// Close is idempotent and releases the underlying lock exactly once.
type Session interface {
	// Issues returns the issues loaded at session open time. Owners read this
	// once per call; the returned slice is treated as read-only.
	Issues() []schema.IssueYaml
	// Save commits one mutated issue back to disk under the session lock.
	// Owner mutates exactly one issue per Transition/Claim, so a single
	// staged-write-and-commit per session is sufficient.
	Save(issue schema.IssueYaml) error
	// Close releases the session lock. Safe to call multiple times.
	Close() error
}
