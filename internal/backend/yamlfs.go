package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// yamlFS implements Backend using local YAML files. All persistence flows
// through a planrepo.Plans repository so the local backend gets the same
// session locking, atomic writes, and full-snapshot validation as the rest
// of the codebase.
type yamlFS struct {
	plans *planrepo.Plans
	cfg   config.Config
}

// NewYAMLFS creates a yaml-fs backend from config.
func NewYAMLFS(cfg config.Config) Backend {
	return &yamlFS{plans: planrepo.NewProd(cfg.PlansDir), cfg: cfg}
}

func (y *yamlFS) CreateProject(_ context.Context, prd *schema.PrdYaml) (RemoteProject, error) {
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

func (y *yamlFS) CreateIssue(_ context.Context, issue *schema.IssueYaml, projectID string) (RemoteIssue, error) {
	sess, err := y.plans.Open(projectID)
	if err != nil {
		return RemoteIssue{}, err
	}
	defer sess.Close()
	if err := upsertIssue(sess, *issue); err != nil {
		return RemoteIssue{}, err
	}
	if err := sess.Commit(y.cfg); err != nil {
		return RemoteIssue{}, err
	}
	return issueToRemote(issue), nil
}

func (y *yamlFS) UpdateIssue(_ context.Context, issue *schema.IssueYaml) (RemoteIssue, error) {
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

func (y *yamlFS) PullIssue(_ context.Context, remoteID string) (RemoteIssue, error) {
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
	snap, err := sess.Snapshot()
	if err != nil {
		return RemoteIssue{}, err
	}
	for i := range snap.Issues {
		iss := &snap.Issues[i]
		if iss.ID == id {
			return issueToRemote(iss), nil
		}
	}
	return RemoteIssue{}, fmt.Errorf("issue not found: %s", remoteID)
}

func (y *yamlFS) PullProject(_ context.Context, projectID string) (PullProjectResult, error) {
	sess, err := y.plans.Open(projectID)
	if err != nil {
		return PullProjectResult{}, err
	}
	defer sess.Close()
	snap, err := sess.Snapshot()
	if err != nil {
		return PullProjectResult{}, err
	}
	remoteIssues := make([]RemoteIssue, len(snap.Issues))
	for i := range snap.Issues {
		remoteIssues[i] = issueToRemote(&snap.Issues[i])
	}
	return PullProjectResult{
		Project: RemoteProject{ID: projectID, Name: snap.PRD.Name},
		Issues:  remoteIssues,
	}, nil
}

// upsertIssue lets CreateIssue act as create-or-update. Existing yamlFS
// callers (notably SyncPush) treat CreateIssue as a write entry point and
// may re-run after a partial failure, so silently routing same-id calls to
// UpdateIssue preserves prior behavior with the session API.
func upsertIssue(sess *planrepo.PlanSession, issue schema.IssueYaml) error {
	snap, err := sess.Snapshot()
	if err != nil {
		return err
	}
	for _, existing := range snap.Issues {
		if existing.ID == issue.ID {
			return sess.UpdateIssue(issue)
		}
	}
	return sess.CreateIssue(issue)
}

func issueToRemote(issue *schema.IssueYaml) RemoteIssue {
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
