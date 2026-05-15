package backend

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

var priorityToLinear = map[string]int{
	"urgent": 1,
	"high":   2,
	"medium": 3,
	"low":    4,
}

var linearToPriority = map[int]string{
	1: "urgent",
	2: "high",
	3: "medium",
	4: "low",
}

type linearBackend struct {
	client            *linear.Client
	cfg               config.Config
	teamID            string
	stateIDs          map[string]string
	estimationEnabled bool
}

func NewLinear(ctx context.Context, cfg config.Config) (Backend, error) {
	if cfg.Linear.APIKey == "" {
		return nil, fmt.Errorf("linear.api_key is required")
	}
	if cfg.Linear.Team == "" {
		return nil, fmt.Errorf("linear.team is required")
	}

	client := linear.NewClient(cfg.Linear.APIKey)

	// Pre-fetch workflow states; also resolves team key → UUID for mutations.
	teamID, states, estimationType, err := client.ListWorkflowStates(ctx, cfg.Linear.Team)
	if err != nil {
		return nil, fmt.Errorf("fetching workflow states: %w", err)
	}

	estimationEnabled := estimationType != "" && estimationType != "notUsed"
	if !estimationEnabled {
		slog.Info("team has estimation disabled; issue points will not be synced", "team", cfg.Linear.Team)
	}

	return &linearBackend{
		client:            client,
		cfg:               cfg,
		teamID:            teamID,
		stateIDs:          states,
		estimationEnabled: estimationEnabled,
	}, nil
}

func (b *linearBackend) CreateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error) {
	project, err := b.client.CreateProject(ctx, prd.Name, b.teamID)
	if err != nil {
		return RemoteProject{}, err
	}
	return RemoteProject{ID: project.ID, Name: project.Name, URL: project.URL}, nil
}

func (b *linearBackend) CreateIssue(ctx context.Context, issue *schema.Issue, projectID, slug string) (RemoteIssue, error) {
	stateID := b.resolveStateID(issue.Status)
	input := linear.IssueCreateInput{
		Title:       issue.Name,
		Description: renderIssueBody(issue, slug),
		TeamID:      b.teamID,
		ProjectID:   projectID,
		Priority:    mapPriority(issue.Priority),
		StateID:     stateID,
	}
	if b.estimationEnabled {
		input.Estimate = issue.Points
	}

	created, err := b.client.CreateIssue(ctx, input)
	if err != nil {
		return RemoteIssue{}, err
	}
	return linearIssueToRemote(created), nil
}

func (b *linearBackend) UpdateIssue(ctx context.Context, issue *schema.Issue, slug string) (RemoteIssue, error) {
	if issue.LinearID == nil || *issue.LinearID == "" {
		return RemoteIssue{}, fmt.Errorf("issue #%d has no linear_id", issue.ID)
	}

	stateID := b.resolveStateID(issue.Status)
	input := linear.IssueUpdateInput{
		Title:       issue.Name,
		Description: renderIssueBody(issue, slug),
		StateID:     stateID,
		Priority:    mapPriority(issue.Priority),
	}
	if b.estimationEnabled {
		input.Estimate = issue.Points
	}

	updated, err := b.client.UpdateIssue(ctx, *issue.LinearID, input)
	if err != nil {
		return RemoteIssue{}, err
	}
	return linearIssueToRemote(updated), nil
}

func (b *linearBackend) PullIssue(ctx context.Context, remoteID string) (RemoteIssue, error) {
	issue, err := b.client.GetIssue(ctx, remoteID)
	if err != nil {
		return RemoteIssue{}, err
	}
	return linearIssueToRemote(issue), nil
}

func (b *linearBackend) PullProject(ctx context.Context, projectID string) (PullProjectResult, error) {
	project, issues, err := b.client.GetProject(ctx, projectID)
	if err != nil {
		return PullProjectResult{}, err
	}

	remoteIssues := make([]RemoteIssue, len(issues))
	for i := range issues {
		remoteIssues[i] = linearIssueToRemote(&issues[i])
	}

	return PullProjectResult{
		Project: RemoteProject{ID: project.ID, Name: project.Name, URL: project.URL},
		Issues:  remoteIssues,
	}, nil
}

func (b *linearBackend) resolveStateID(status string) string {
	if b.cfg.Linear.StatusMap != nil {
		if mapped, ok := b.cfg.Linear.StatusMap[status]; ok {
			if id, ok := b.stateIDs[mapped]; ok {
				return id
			}
			slog.Warn("status_map references unknown Linear state", "status", status, "mapped_to", mapped)
		}
	}

	for name, id := range b.stateIDs {
		if strings.EqualFold(name, status) {
			return id
		}
	}

	slog.Warn("no matching Linear state for status", "status", status)
	return ""
}

func mapPriority(priority string) int {
	if p, ok := priorityToLinear[priority]; ok {
		return p
	}
	return 3
}

func ReversePriority(priority int) string {
	if p, ok := linearToPriority[priority]; ok {
		return p
	}
	return "medium"
}

func linearIssueToRemote(issue *linear.Issue) RemoteIssue {
	var labels []string
	for _, l := range issue.Labels.Nodes {
		labels = append(labels, l.Name)
	}

	assignee := ""
	if issue.Assignee != nil {
		assignee = issue.Assignee.Name
	}

	return RemoteIssue{
		ID:       issue.ID,
		Title:    issue.Title,
		Status:   issue.State.Name,
		Priority: ReversePriority(int(issue.Priority)),
		Labels:   labels,
		Assignee: assignee,
		URL:      issue.URL,
	}
}
