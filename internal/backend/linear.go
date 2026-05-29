package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
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
	root              string // project root, re-read on each op to re-check linear.enabled
	teamID            string
	stateIDs          map[string]string
	labelIDs          map[string]string // lowercased label name → Linear label id; nil until first load
	estimationEnabled bool
}

func NewLinear(ctx context.Context, root string, cfg config.Config) (Backend, error) {
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
		root:              root,
		teamID:            teamID,
		stateIDs:          states,
		estimationEnabled: estimationEnabled,
	}, nil
}

// ensureEnabled re-reads the project .plan-bender.json from disk and fails if
// it explicitly sets linear.enabled:false. Checking on every read and write
// makes the flag a live kill-switch: flip it off in the file and the next
// Linear operation refuses rather than continuing on the value the backend was
// built with. The file's explicit flag is read directly (not the merged
// config) because mergeLinear only ever turns enabled on — a project-level
// false could never override a global enable through the normal merge. When
// the flag is absent we fall back to the constructed config; when root is
// empty (tests building the backend directly) the check is skipped.
func (b *linearBackend) ensureEnabled() error {
	if b.root == "" {
		return nil
	}
	enabled := b.cfg.Linear.Enabled

	data, err := os.ReadFile(filepath.Join(b.root, ".plan-bender.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// no project file — keep the constructed value
	case err != nil:
		return fmt.Errorf("re-checking linear config: %w", err)
	default:
		var doc struct {
			Linear struct {
				Enabled *bool `json:"enabled"`
			} `json:"linear"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("re-checking linear config: %w", err)
		}
		if doc.Linear.Enabled != nil {
			enabled = *doc.Linear.Enabled
		}
	}

	if !enabled {
		return fmt.Errorf("linear is disabled in .plan-bender.json")
	}
	return nil
}

func (b *linearBackend) CreateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error) {
	if err := b.ensureEnabled(); err != nil {
		return RemoteProject{}, err
	}
	description, content := renderProjectBody(prd)
	project, err := b.client.CreateProject(ctx, linear.ProjectCreateInput{
		Name:        prd.Name,
		TeamIDs:     []string{b.teamID},
		Description: description,
		Content:     content,
	})
	if err != nil {
		return RemoteProject{}, err
	}
	return RemoteProject{ID: project.ID, Name: project.Name, URL: project.URL}, nil
}

func (b *linearBackend) UpdateProject(ctx context.Context, prd *schema.PRD) (RemoteProject, error) {
	if err := b.ensureEnabled(); err != nil {
		return RemoteProject{}, err
	}
	if prd.Linear == nil || prd.Linear.ProjectID == "" {
		return RemoteProject{}, fmt.Errorf("PRD has no linear project_id")
	}
	description, content := renderProjectBody(prd)
	project, err := b.client.UpdateProject(ctx, prd.Linear.ProjectID, linear.ProjectUpdateInput{
		Description: description,
		Content:     content,
	})
	if err != nil {
		return RemoteProject{}, err
	}
	return RemoteProject{ID: project.ID, Name: project.Name, URL: project.URL}, nil
}

func (b *linearBackend) CreateIssue(ctx context.Context, issue *schema.Issue, projectID, slug string) (RemoteIssue, error) {
	if err := b.ensureEnabled(); err != nil {
		return RemoteIssue{}, err
	}
	labelIDs, err := b.resolveLabels(ctx, issue.Labels)
	if err != nil {
		return RemoteIssue{}, err
	}

	stateID := b.resolveStateID(issue.Status)
	input := linear.IssueCreateInput{
		Title:       issue.Name,
		Description: renderIssueBody(issue, slug),
		TeamID:      b.teamID,
		ProjectID:   projectID,
		Priority:    mapPriority(issue.Priority),
		StateID:     stateID,
		LabelIDs:    labelIDs,
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
	if err := b.ensureEnabled(); err != nil {
		return RemoteIssue{}, err
	}
	if issue.LinearID == nil || *issue.LinearID == "" {
		return RemoteIssue{}, fmt.Errorf("issue #%d has no linear_id", issue.ID)
	}

	labelIDs, err := b.resolveLabels(ctx, issue.Labels)
	if err != nil {
		return RemoteIssue{}, err
	}

	stateID := b.resolveStateID(issue.Status)
	input := linear.IssueUpdateInput{
		Title:       issue.Name,
		Description: renderIssueBody(issue, slug),
		StateID:     stateID,
		Priority:    mapPriority(issue.Priority),
		LabelIDs:    labelIDs,
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
	if err := b.ensureEnabled(); err != nil {
		return RemoteIssue{}, err
	}
	issue, err := b.client.GetIssue(ctx, remoteID)
	if err != nil {
		return RemoteIssue{}, err
	}
	return linearIssueToRemote(issue), nil
}

func (b *linearBackend) PullProject(ctx context.Context, projectID string) (PullProjectResult, error) {
	if err := b.ensureEnabled(); err != nil {
		return PullProjectResult{}, err
	}
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

// resolveLabels maps plan label names to Linear label ids, creating any label
// missing from the team. Lookup is case-insensitive: the cache is keyed on the
// lowercased name so "HITL" and "hitl" resolve to the same label.
func (b *linearBackend) resolveLabels(ctx context.Context, labels []string) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}

	if b.labelIDs == nil {
		existing, err := b.client.ListIssueLabels(ctx, b.teamID)
		if err != nil {
			return nil, fmt.Errorf("listing issue labels: %w", err)
		}
		b.labelIDs = make(map[string]string, len(existing))
		for _, l := range existing {
			b.labelIDs[strings.ToLower(l.Name)] = l.ID
		}
	}

	ids := make([]string, 0, len(labels))
	for _, name := range labels {
		key := strings.ToLower(name)
		id, ok := b.labelIDs[key]
		if !ok {
			created, err := b.client.CreateIssueLabel(ctx, b.teamID, name)
			if err != nil {
				return nil, fmt.Errorf("creating issue label %q: %w", name, err)
			}
			id = created.ID
			b.labelIDs[key] = id
		}
		ids = append(ids, id)
	}
	return ids, nil
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
