package status

import "github.com/jasonraimondi/plan-bender/internal/schema"

// Store is the narrow persistence interface required by Owner. Kept minimal
// so package tests can supply an in-memory fake without dragging in the
// backend package, and so the production adapter (issue #2) is a thin
// wrapper over backend.PlanStore.
type Store interface {
	Load(slug string) ([]schema.IssueYaml, error)
	Save(slug string, issue schema.IssueYaml) error
	Lock(slug string) (release func() error, err error)
}
