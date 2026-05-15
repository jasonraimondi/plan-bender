package backend

import (
	"context"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

type RemoteProject struct {
	ID   string
	Name string
	URL  string
}

type RemoteIssue struct {
	ID       string
	Title    string
	Status   string
	Priority string
	Labels   []string
	Assignee string
	URL      string
}

type PullProjectResult struct {
	Project RemoteProject
	Issues  []RemoteIssue
}

type Backend interface {
	CreateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error)
	UpdateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error)
	CreateIssue(ctx context.Context, issue *schema.Issue, projectID, slug string) (RemoteIssue, error)
	UpdateIssue(ctx context.Context, issue *schema.Issue, slug string) (RemoteIssue, error)
	PullIssue(ctx context.Context, remoteID string) (RemoteIssue, error)
	PullProject(ctx context.Context, projectID string) (PullProjectResult, error)
}
