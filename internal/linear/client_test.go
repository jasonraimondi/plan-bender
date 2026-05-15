package linear

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	client := NewClient("lin_api_test_key")
	assert.NotNil(t, client)
	assert.NotNil(t, client.gql)
}

func TestNewClientWithHTTP(t *testing.T) {
	client := NewClientWithHTTP(&http.Client{})
	assert.NotNil(t, client)
	assert.NotNil(t, client.gql)
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func clientWithResponse(body string) *Client {
	return NewClientWithHTTP(&http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(body), nil
		}),
	})
}

func TestCreateProject(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"projectCreate": {
				"success": true,
				"project": {
					"id": "proj-123",
					"name": "Test Project",
					"url": "https://linear.app/team/project/proj-123"
				}
			}
		}
	}`)

	project, err := c.CreateProject(t.Context(), ProjectCreateInput{
		Name:        "Test Project",
		TeamIDs:     []string{"team-1"},
		Description: "Short description.",
		Content:     "## Why\n\nFull body.",
	})
	require.NoError(t, err)
	assert.Equal(t, "proj-123", project.ID)
	assert.Equal(t, "Test Project", project.Name)
	assert.Equal(t, "https://linear.app/team/project/proj-123", project.URL)
}

func TestUpdateProject(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"projectUpdate": {
				"success": true,
				"project": {
					"id": "proj-123",
					"name": "Test Project",
					"url": "https://linear.app/team/project/proj-123"
				}
			}
		}
	}`)

	project, err := c.UpdateProject(t.Context(), "proj-123", ProjectUpdateInput{
		Description: "Refreshed description.",
		Content:     "## Why\n\nRefreshed body.",
	})
	require.NoError(t, err)
	assert.Equal(t, "proj-123", project.ID)
	assert.Equal(t, "Test Project", project.Name)
}

func TestUpdateProject_SuccessFalse(t *testing.T) {
	c := clientWithResponse(`{"data": {"projectUpdate": {"success": false, "project": {"id": "", "name": "", "url": ""}}}}`)

	_, err := c.UpdateProject(t.Context(), "proj-123", ProjectUpdateInput{Content: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "success=false")
}

func TestCreateIssue(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"issueCreate": {
				"success": true,
				"issue": {
					"id": "issue-456",
					"title": "Test Issue",
					"description": "Test description",
					"state": {"name": "Backlog"},
					"priority": 2,
					"labels": {"nodes": [{"name": "bug"}, {"name": "p0"}]},
					"assignee": {"name": "alice"},
					"url": "https://linear.app/issue/issue-456"
				}
			}
		}
	}`)

	issue, err := c.CreateIssue(t.Context(), IssueCreateInput{
		Title:     "Test Issue",
		TeamID:    "team-1",
		ProjectID: "proj-1",
		Priority:  2,
	})
	require.NoError(t, err)
	assert.Equal(t, "issue-456", issue.ID)
	assert.Equal(t, "Test Issue", issue.Title)
	assert.Equal(t, "Backlog", issue.State.Name)
	assert.Equal(t, float64(2), issue.Priority)
	require.Len(t, issue.Labels.Nodes, 2)
	assert.Equal(t, "bug", issue.Labels.Nodes[0].Name)
	assert.Equal(t, "p0", issue.Labels.Nodes[1].Name)
	require.NotNil(t, issue.Assignee)
	assert.Equal(t, "alice", issue.Assignee.Name)
}

func TestUpdateIssue(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"issueUpdate": {
				"success": true,
				"issue": {
					"id": "issue-456",
					"title": "Updated Title",
					"description": "",
					"state": {"name": "In Progress"},
					"priority": 1,
					"labels": {"nodes": []},
					"assignee": null,
					"url": "https://linear.app/issue/issue-456"
				}
			}
		}
	}`)

	issue, err := c.UpdateIssue(t.Context(), "issue-456", IssueUpdateInput{
		Title:   "Updated Title",
		StateID: "state-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "issue-456", issue.ID)
	assert.Equal(t, "Updated Title", issue.Title)
	assert.Equal(t, "In Progress", issue.State.Name)
	assert.Equal(t, float64(1), issue.Priority)
	assert.Nil(t, issue.Assignee)
}

func TestListIssueLabels(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"team": {
				"labels": {
					"nodes": [
						{"id": "label-1", "name": "AFK"},
						{"id": "label-2", "name": "HITL"}
					]
				}
			}
		}
	}`)

	labels, err := c.ListIssueLabels(t.Context(), "team-1")
	require.NoError(t, err)
	require.Len(t, labels, 2)
	assert.Equal(t, "label-1", labels[0].ID)
	assert.Equal(t, "AFK", labels[0].Name)
	assert.Equal(t, "label-2", labels[1].ID)
	assert.Equal(t, "HITL", labels[1].Name)
}

func TestCreateIssueLabel(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"issueLabelCreate": {
				"success": true,
				"issueLabel": {"id": "label-9", "name": "AFK"}
			}
		}
	}`)

	label, err := c.CreateIssueLabel(t.Context(), "team-1", "AFK")
	require.NoError(t, err)
	assert.Equal(t, "label-9", label.ID)
	assert.Equal(t, "AFK", label.Name)
}

func TestCreateIssueLabel_Failure(t *testing.T) {
	c := clientWithResponse(`{
		"data": {"issueLabelCreate": {"success": false, "issueLabel": {"id": "", "name": ""}}}
	}`)

	_, err := c.CreateIssueLabel(t.Context(), "team-1", "AFK")
	require.Error(t, err)
}

func TestListWorkflowStates_EstimationEnabled(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"team": {
				"id": "team-uuid-1",
				"issueEstimationType": "fibonacci",
				"states": {
					"nodes": [
						{"id": "state-1", "name": "Backlog"},
						{"id": "state-2", "name": "Done"}
					]
				}
			}
		}
	}`)

	teamID, states, estimationType, err := c.ListWorkflowStates(t.Context(), "ENG")
	require.NoError(t, err)
	assert.Equal(t, "team-uuid-1", teamID)
	assert.Equal(t, "fibonacci", estimationType)
	assert.Equal(t, map[string]string{"Backlog": "state-1", "Done": "state-2"}, states)
}

func TestListWorkflowStates_EstimationDisabled(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"team": {
				"id": "team-uuid-1",
				"issueEstimationType": "notUsed",
				"states": {"nodes": []}
			}
		}
	}`)

	_, _, estimationType, err := c.ListWorkflowStates(t.Context(), "ENG")
	require.NoError(t, err)
	assert.Equal(t, "notUsed", estimationType)
}

func TestGetProject(t *testing.T) {
	c := clientWithResponse(`{
		"data": {
			"project": {
				"id": "proj-123",
				"name": "Test Project",
				"url": "https://linear.app/project/proj-123",
				"issues": {
					"nodes": [
						{
							"id": "issue-1",
							"title": "First Issue",
							"description": "desc",
							"state": {"name": "Backlog"},
							"priority": 3,
							"labels": {"nodes": [{"name": "feature"}]},
							"assignee": {"name": "bob"},
							"url": "https://linear.app/issue/issue-1"
						},
						{
							"id": "issue-2",
							"title": "Second Issue",
							"description": "",
							"state": {"name": "Done"},
							"priority": 4,
							"labels": {"nodes": []},
							"assignee": null,
							"url": "https://linear.app/issue/issue-2"
						}
					]
				}
			}
		}
	}`)

	project, issues, err := c.GetProject(t.Context(), "proj-123")
	require.NoError(t, err)

	assert.Equal(t, "proj-123", project.ID)
	assert.Equal(t, "Test Project", project.Name)

	require.Len(t, issues, 2)
	assert.Equal(t, "issue-1", issues[0].ID)
	assert.Equal(t, "First Issue", issues[0].Title)
	assert.Equal(t, "Backlog", issues[0].State.Name)
	require.NotNil(t, issues[0].Assignee)
	assert.Equal(t, "bob", issues[0].Assignee.Name)

	assert.Equal(t, "issue-2", issues[1].ID)
	assert.Equal(t, "Done", issues[1].State.Name)
	assert.Nil(t, issues[1].Assignee)
}
