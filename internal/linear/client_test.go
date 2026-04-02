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

	project, err := c.CreateProject(t.Context(), "Test Project", "team-1")
	require.NoError(t, err)
	assert.Equal(t, "proj-123", project.ID)
	assert.Equal(t, "Test Project", project.Name)
	assert.Equal(t, "https://linear.app/team/project/proj-123", project.URL)
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
