package backend

import (
	"context"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// RemoteProject represents a project in the backend.
type RemoteProject struct {
	ID   string
	Name string
	URL  string
}

// RemoteIssue represents an issue in the backend.
type RemoteIssue struct {
	ID       string
	Title    string
	Status   string
	Priority string
	Labels   []string
	Assignee string
	URL      string
}

// PullProjectResult contains a project and its issues.
type PullProjectResult struct {
	Project RemoteProject
	Issues  []RemoteIssue
}

// Backend is the tracking backend interface.
type Backend interface {
	CreateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error)
	CreateIssue(ctx context.Context, issue *schema.Issue, projectID string) (RemoteIssue, error)
	UpdateIssue(ctx context.Context, issue *schema.Issue) (RemoteIssue, error)
	PullIssue(ctx context.Context, remoteID string) (RemoteIssue, error)
	PullProject(ctx context.Context, projectID string) (PullProjectResult, error)
}
