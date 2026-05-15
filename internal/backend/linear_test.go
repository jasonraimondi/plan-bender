package backend

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureTransport records the outgoing request body and replays a canned response.
type captureTransport struct {
	body     string
	response string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	t.body = string(b)
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(t.response)),
	}, nil
}

func backendWithCapture(response string, estimationEnabled bool) (*linearBackend, *captureTransport) {
	ct := &captureTransport{response: response}
	client := linear.NewClientWithHTTP(&http.Client{Transport: ct})
	b := &linearBackend{
		client:            client,
		cfg:               config.Defaults(),
		teamID:            "team-1",
		stateIDs:          map[string]string{"Backlog": "state-1"},
		estimationEnabled: estimationEnabled,
	}
	return b, ct
}

func TestMapPriority(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"urgent", 1},
		{"high", 2},
		{"medium", 3},
		{"low", 4},
		{"unknown", 3},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, mapPriority(tt.in), "mapPriority(%q)", tt.in)
	}
}

func TestReversePriority(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{1, "urgent"},
		{2, "high"},
		{3, "medium"},
		{4, "low"},
		{0, "medium"},
		{99, "medium"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ReversePriority(tt.in), "ReversePriority(%d)", tt.in)
	}
}

func TestResolveStateID(t *testing.T) {
	b := &linearBackend{
		cfg: func() config.Config {
			c := config.Defaults()
			c.Linear.StatusMap = map[string]string{"in-progress": "In Progress"}
			return c
		}(),
		stateIDs: map[string]string{
			"In Progress": "state-1",
			"Backlog":     "state-2",
			"Done":        "state-3",
		},
	}

	assert.Equal(t, "state-1", b.resolveStateID("in-progress"))

	assert.Equal(t, "state-2", b.resolveStateID("backlog"))
	assert.Equal(t, "state-3", b.resolveStateID("done"))

	assert.Equal(t, "", b.resolveStateID("nonexistent"))
}

func TestLinearIssueToRemote_WithAssignee(t *testing.T) {
	issue := &linear.Issue{
		ID:       "lin-1",
		Title:    "Test",
		Priority: 2,
		URL:      "https://linear.app/issue/lin-1",
	}
	issue.State.Name = "In Progress"
	issue.Labels.Nodes = []struct{ Name string }{{Name: "bug"}, {Name: "p0"}}
	issue.Assignee = &struct{ Name string }{Name: "alice"}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, "lin-1", remote.ID)
	assert.Equal(t, "Test", remote.Title)
	assert.Equal(t, "In Progress", remote.Status)
	assert.Equal(t, "high", remote.Priority)
	assert.Equal(t, []string{"bug", "p0"}, remote.Labels)
	assert.Equal(t, "alice", remote.Assignee)
	assert.Equal(t, "https://linear.app/issue/lin-1", remote.URL)
}

func TestLinearIssueToRemote_NilAssignee(t *testing.T) {
	issue := &linear.Issue{
		ID:       "lin-2",
		Title:    "No Assignee",
		Priority: 0,
	}
	issue.State.Name = "Backlog"

	remote := linearIssueToRemote(issue)
	assert.Equal(t, "", remote.Assignee)
	assert.Equal(t, "medium", remote.Priority)
	assert.Nil(t, remote.Labels)
}

const createIssueResponse = `{"data":{"issueCreate":{"success":true,"issue":{"id":"i1","title":"T","state":{"name":"Backlog"}}}}}`
const updateIssueResponse = `{"data":{"issueUpdate":{"success":true,"issue":{"id":"i1","title":"T","state":{"name":"Backlog"}}}}}`

func TestCreateIssue_EstimationEnabled(t *testing.T) {
	b, ct := backendWithCapture(createIssueResponse, true)
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 5}

	_, err := b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.NoError(t, err)
	assert.Contains(t, ct.body, `"estimate":5`)
}

func TestCreateIssue_EstimationDisabled(t *testing.T) {
	b, ct := backendWithCapture(createIssueResponse, false)
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 5}

	_, err := b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.NoError(t, err)
	assert.NotContains(t, ct.body, "estimate")
}

func TestUpdateIssue_EstimationEnabled(t *testing.T) {
	b, ct := backendWithCapture(updateIssueResponse, true)
	linearID := "lin-1"
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 3, LinearID: &linearID}

	_, err := b.UpdateIssue(t.Context(), issue, "slug")
	require.NoError(t, err)
	assert.Contains(t, ct.body, `"estimate":3`)
}

func TestUpdateIssue_EstimationDisabled(t *testing.T) {
	b, ct := backendWithCapture(updateIssueResponse, false)
	linearID := "lin-1"
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 3, LinearID: &linearID}

	_, err := b.UpdateIssue(t.Context(), issue, "slug")
	require.NoError(t, err)
	assert.NotContains(t, ct.body, "estimate")
}

func TestLinearIssueToRemote_MultipleLabels(t *testing.T) {
	issue := &linear.Issue{ID: "lin-3"}
	issue.Labels.Nodes = []struct{ Name string }{
		{Name: "feature"}, {Name: "frontend"}, {Name: "urgent"},
	}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, []string{"feature", "frontend", "urgent"}, remote.Labels)
}
