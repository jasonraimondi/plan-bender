package backend

import (
	"context"
	"fmt"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// yamlFS implements Backend using local YAML files.
type yamlFS struct {
	store *PlanStore
}

// NewYAMLFS creates a yaml-fs backend from config.
func NewYAMLFS(cfg config.Config) Backend {
	return &yamlFS{store: NewProdPlanStore(cfg.PlansDir)}
}

// newYAMLFSWithStore creates a yaml-fs backend with a custom store (for testing).
func newYAMLFSWithStore(store *PlanStore) Backend {
	return &yamlFS{store: store}
}

func (y *yamlFS) CreateProject(_ context.Context, prd *schema.PrdYaml) (RemoteProject, error) {
	if err := y.store.WritePrd(prd.Slug, prd); err != nil {
		return RemoteProject{}, err
	}
	return RemoteProject{ID: prd.Slug, Name: prd.Name}, nil
}

func (y *yamlFS) CreateIssue(_ context.Context, issue *schema.IssueYaml, projectID string) (RemoteIssue, error) {
	if err := y.store.WriteIssue(projectID, issue); err != nil {
		return RemoteIssue{}, err
	}
	return issueToRemote(issue), nil
}

func (y *yamlFS) UpdateIssue(_ context.Context, issue *schema.IssueYaml) (RemoteIssue, error) {
	// Scan all projects to find which one owns this issue
	slug, err := y.findProjectForIssue(issue.ID)
	if err != nil {
		return RemoteIssue{}, err
	}
	if err := y.store.WriteIssue(slug, issue); err != nil {
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

	issues, err := y.store.ReadIssues(slug)
	if err != nil {
		return RemoteIssue{}, err
	}
	for i := range issues {
		if issues[i].ID == id {
			return issueToRemote(&issues[i]), nil
		}
	}
	return RemoteIssue{}, fmt.Errorf("issue not found: %s", remoteID)
}

func (y *yamlFS) PullProject(_ context.Context, projectID string) (PullProjectResult, error) {
	prd, err := y.store.ReadPrd(projectID)
	if err != nil {
		return PullProjectResult{}, err
	}
	issues, err := y.store.ReadIssues(projectID)
	if err != nil {
		return PullProjectResult{}, err
	}

	remoteIssues := make([]RemoteIssue, len(issues))
	for i := range issues {
		remoteIssues[i] = issueToRemote(&issues[i])
	}

	return PullProjectResult{
		Project: RemoteProject{ID: projectID, Name: prd.Name},
		Issues:  remoteIssues,
	}, nil
}

func (y *yamlFS) findProjectForIssue(issueID int) (string, error) {
	// This is a simple scan — fine for local file systems
	entries, err := readDirSafe(y.store.fsys, ".")
	if err != nil {
		return "", fmt.Errorf("scanning plans: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		_, err := y.store.FindIssueFile(e.Name(), issueID)
		if err == nil {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("cannot find project for issue #%d", issueID)
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
