package linear

import (
	"context"
	"fmt"
	"net/http"

	graphql "github.com/hasura/go-graphql-client"
)

const linearAPIURL = "https://api.linear.app/graphql"

// Client wraps the Linear GraphQL API.
type Client struct {
	gql *graphql.Client
}

// NewClient creates a Linear API client with the given API key.
func NewClient(apiKey string) *Client {
	httpClient := &http.Client{
		Transport: &authTransport{
			apiKey: apiKey,
			base:   http.DefaultTransport,
		},
	}
	return NewClientWithHTTP(httpClient)
}

// NewClientWithHTTP creates a Linear client with a custom http.Client (for testing).
func NewClientWithHTTP(httpClient *http.Client) *Client {
	gql := graphql.NewClient(linearAPIURL, httpClient)
	return &Client{gql: gql}
}

type authTransport struct {
	apiKey string
	base   http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", t.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return t.base.RoundTrip(req)
}

// Project types
type Project struct {
	ID   string
	Name string
	URL  string
}

type Issue struct {
	ID          string
	Title       string
	Description string
	State       struct{ Name string }
	Priority    float64
	Labels      struct {
		Nodes []struct{ Name string }
	}
	Assignee *struct{ Name string }
	URL      string
}

// CreateProject creates a new Linear project.
func (c *Client) CreateProject(ctx context.Context, name, teamID string) (*Project, error) {
	var mutation struct {
		ProjectCreate struct {
			Success bool
			Project struct {
				ID   string
				Name string
				URL  string `graphql:"url"`
			}
		} `graphql:"projectCreate(input: {name: $name, teamIds: [$teamID]})"`
	}

	vars := map[string]any{
		"name":   graphql.String(name),
		"teamID": graphql.String(teamID),
	}

	if err := c.gql.Mutate(ctx, &mutation, vars); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	if !mutation.ProjectCreate.Success {
		return nil, fmt.Errorf("creating project: mutation returned success=false")
	}

	return &Project{
		ID:   mutation.ProjectCreate.Project.ID,
		Name: mutation.ProjectCreate.Project.Name,
		URL:  mutation.ProjectCreate.Project.URL,
	}, nil
}

// CreateIssue creates a new Linear issue.
func (c *Client) CreateIssue(ctx context.Context, input IssueCreateInput) (*Issue, error) {
	var mutation struct {
		IssueCreate struct {
			Success bool
			Issue   Issue
		} `graphql:"issueCreate(input: $input)"`
	}

	vars := map[string]any{
		"input": input,
	}

	if err := c.gql.Mutate(ctx, &mutation, vars); err != nil {
		return nil, fmt.Errorf("creating issue: %w", err)
	}
	return &mutation.IssueCreate.Issue, nil
}

// IssueCreateInput is the input for creating a Linear issue.
type IssueCreateInput struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	TeamID      string     `json:"teamId"`
	ProjectID   string     `json:"projectId,omitempty"`
	Priority    int        `json:"priority,omitempty"`
	StateID     string     `json:"stateId,omitempty"`
}

// UpdateIssue updates a Linear issue.
func (c *Client) UpdateIssue(ctx context.Context, issueID string, input IssueUpdateInput) (*Issue, error) {
	var mutation struct {
		IssueUpdate struct {
			Success bool
			Issue   Issue
		} `graphql:"issueUpdate(id: $id, input: $input)"`
	}

	vars := map[string]any{
		"id":    graphql.ID(issueID),
		"input": input,
	}

	if err := c.gql.Mutate(ctx, &mutation, vars); err != nil {
		return nil, fmt.Errorf("updating issue: %w", err)
	}
	return &mutation.IssueUpdate.Issue, nil
}

// IssueUpdateInput is the input for updating a Linear issue.
type IssueUpdateInput struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	StateID     string `json:"stateId,omitempty"`
	Priority    int    `json:"priority,omitempty"`
}

// GetIssue fetches a Linear issue by ID.
func (c *Client) GetIssue(ctx context.Context, issueID string) (*Issue, error) {
	var query struct {
		Issue Issue `graphql:"issue(id: $id)"`
	}

	vars := map[string]any{
		"id": graphql.String(issueID),
	}

	if err := c.gql.Query(ctx, &query, vars); err != nil {
		return nil, fmt.Errorf("fetching issue: %w", err)
	}
	return &query.Issue, nil
}

// GetProject fetches a Linear project with its issues.
func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, []Issue, error) {
	var query struct {
		Project struct {
			ID     string
			Name   string
			URL    string `graphql:"url"`
			Issues struct {
				Nodes []Issue
			}
		} `graphql:"project(id: $id)"`
	}

	vars := map[string]any{
		"id": graphql.String(projectID),
	}

	if err := c.gql.Query(ctx, &query, vars); err != nil {
		return nil, nil, fmt.Errorf("fetching project: %w", err)
	}

	project := &Project{
		ID:   query.Project.ID,
		Name: query.Project.Name,
		URL:  query.Project.URL,
	}
	return project, query.Project.Issues.Nodes, nil
}

// ListWorkflowStates fetches the resolved team UUID and workflow states for a team.
// teamKey may be a team key (e.g. "ENG") or a UUID; the returned resolvedID is always a UUID.
func (c *Client) ListWorkflowStates(ctx context.Context, teamKey string) (resolvedID string, states map[string]string, err error) {
	var query struct {
		Team struct {
			ID     string
			States struct {
				Nodes []struct {
					ID   string
					Name string
				}
			}
		} `graphql:"team(id: $id)"`
	}

	vars := map[string]any{
		"id": graphql.String(teamKey),
	}

	if err := c.gql.Query(ctx, &query, vars); err != nil {
		return "", nil, fmt.Errorf("fetching workflow states: %w", err)
	}

	states = make(map[string]string)
	for _, s := range query.Team.States.Nodes {
		states[s.Name] = s.ID
	}
	return query.Team.ID, states, nil
}

// CreateAttachment creates a URL attachment on an issue.
func (c *Client) CreateAttachment(ctx context.Context, issueID, url, title string) error {
	var mutation struct {
		AttachmentCreate struct {
			Success bool
		} `graphql:"attachmentCreate(input: {issueId: $issueId, url: $url, title: $title})"`
	}

	vars := map[string]any{
		"issueId": graphql.ID(issueID),
		"url":     graphql.String(url),
		"title":   graphql.String(title),
	}

	if err := c.gql.Mutate(ctx, &mutation, vars); err != nil {
		return fmt.Errorf("creating attachment: %w", err)
	}
	return nil
}
