package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// localFS implements Backend using local JSON files. All persistence flows
// through a planrepo.Plans repository so the local backend gets the same
// session locking, atomic writes, and full-snapshot validation as the rest
// of the codebase.
type localFS struct {
	plans *planrepo.Plans
	cfg   config.Config
}

func NewLocalFS(cfg config.Config) Backend {
	return &localFS{plans: planrepo.NewProd(cfg.PlansDir), cfg: cfg}
}

func (y *localFS) CreateProject(_ context.Context, prd *schema.PRD) (RemoteProject, error) {
	sess, err := y.plans.OpenOrCreate(prd.Slug)
	if err != nil {
		return RemoteProject{}, err
	}
	defer sess.Close()
	if err := sess.UpdatePrd(*prd); err != nil {
		return RemoteProject{}, err
	}
	if err := sess.Commit(y.cfg); err != nil {
		return RemoteProject{}, err
	}
	return RemoteProject{ID: prd.Slug, Name: prd.Name}, nil
}

func (y *localFS) CreateIssue(_ context.Context, issue *schema.Issue, projectID, _ string) (RemoteIssue, error) {
	sess, err := y.plans.Open(projectID)
	if err != nil {
		return RemoteIssue{}, err
	}
	defer sess.Close()
	found := false
	for _, existing := range sess.Snapshot().Issues {
		if existing.ID == issue.ID {
			found = true
			break
		}
	}
	if found {
		if err := sess.UpdateIssue(*issue); err != nil {
			return RemoteIssue{}, err
		}
	} else if err := sess.CreateIssue(*issue); err != nil {
		return RemoteIssue{}, err
	}
	if err := sess.Commit(y.cfg); err != nil {
		return RemoteIssue{}, err
	}
	return issueToRemote(issue), nil
}

func (y *localFS) UpdateIssue(_ context.Context, issue *schema.Issue, _ string) (RemoteIssue, error) {
	slug, err := y.plans.FindIssueProject(issue.ID)
	if err != nil {
		return RemoteIssue{}, err
	}
	sess, err := y.plans.Open(slug)
	if err != nil {
		return RemoteIssue{}, err
	}
	defer sess.Close()
	if err := sess.UpdateIssue(*issue); err != nil {
		return RemoteIssue{}, err
	}
	if err := sess.Commit(y.cfg); err != nil {
		return RemoteIssue{}, err
	}
	return issueToRemote(issue), nil
}

func (y *localFS) PullIssue(_ context.Context, remoteID string) (RemoteIssue, error) {
	parts := strings.SplitN(remoteID, "/", 2)
	if len(parts) != 2 {
		return RemoteIssue{}, fmt.Errorf("invalid remoteId format: %s", remoteID)
	}
	slug := parts[0]
	var id int
	if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
		return RemoteIssue{}, fmt.Errorf("invalid issue id in remoteId: %s", remoteID)
	}

	sess, err := y.plans.Open(slug)
	if err != nil {
		return RemoteIssue{}, err
	}
	defer sess.Close()
	for i := range sess.Snapshot().Issues {
		iss := &sess.Snapshot().Issues[i]
		if iss.ID == id {
			return issueToRemote(iss), nil
		}
	}
	return RemoteIssue{}, fmt.Errorf("issue not found: %s", remoteID)
}

func (y *localFS) PullProject(_ context.Context, projectID string) (PullProjectResult, error) {
	sess, err := y.plans.Open(projectID)
	if err != nil {
		return PullProjectResult{}, err
	}
	defer sess.Close()
	snap := sess.Snapshot()
	remoteIssues := make([]RemoteIssue, len(snap.Issues))
	for i := range snap.Issues {
		remoteIssues[i] = issueToRemote(&snap.Issues[i])
	}
	return PullProjectResult{
		Project: RemoteProject{ID: projectID, Name: snap.PRD.Name},
		Issues:  remoteIssues,
	}, nil
}

func issueToRemote(issue *schema.Issue) RemoteIssue {
	assignee := ""
	if issue.Assignee != nil {
		assignee = *issue.Assignee
	}
	return RemoteIssue{
		ID:       fmt.Sprintf("%d", issue.ID),
		Title:    issue.Name,
		Status:   issue.Status,
		Priority: issue.Priority,
		Labels:   issue.Labels,
		Assignee: assignee,
	}
}
