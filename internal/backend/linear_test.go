package backend

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/linear"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// routingClient returns a Linear client whose responses are selected by
// matching a substring of the GraphQL request body against routes.
func routingClient(t *testing.T, routes map[string]string) *linear.Client {
	t.Helper()
	return linear.NewClientWithHTTP(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			for substr, resp := range routes {
				if strings.Contains(string(body), substr) {
					return &http.Response{
						StatusCode: 200,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(bytes.NewBufferString(resp)),
					}, nil
				}
			}
			t.Fatalf("no route for request body: %s", body)
			return nil, nil
		}),
	})
}

func TestResolveLabels_Empty(t *testing.T) {
	b := &linearBackend{teamID: "team-1"}
	ids, err := b.resolveLabels(t.Context(), nil)
	require.NoError(t, err)
	assert.Nil(t, ids)
}

func TestResolveLabels_CaseInsensitive(t *testing.T) {
	// Only a labels-list route — a create attempt would Fatalf.
	b := &linearBackend{
		teamID: "team-1",
		client: routingClient(t, map[string]string{
			"labels": `{"data":{"team":{"labels":{"nodes":[{"id":"label-1","name":"HITL"}]}}}}`,
		}),
	}

	ids, err := b.resolveLabels(t.Context(), []string{"hitl"})
	require.NoError(t, err)
	assert.Equal(t, []string{"label-1"}, ids)
}

func TestResolveLabels_CreateOnMiss(t *testing.T) {
	b := &linearBackend{
		teamID: "team-1",
		client: routingClient(t, map[string]string{
			"labels":           `{"data":{"team":{"labels":{"nodes":[{"id":"label-1","name":"AFK"}]}}}}`,
			"issueLabelCreate": `{"data":{"issueLabelCreate":{"success":true,"issueLabel":{"id":"label-9","name":"HITL"}}}}`,
		}),
	}

	ids, err := b.resolveLabels(t.Context(), []string{"AFK", "HITL"})
	require.NoError(t, err)
	assert.Equal(t, []string{"label-1", "label-9"}, ids)

	// The freshly created label is cached for subsequent lookups.
	assert.Equal(t, "label-9", b.labelIDs["hitl"])
}

func TestLinearIssueToRemote_MultipleLabels(t *testing.T) {
	issue := &linear.Issue{ID: "lin-3"}
	issue.Labels.Nodes = []struct{ Name string }{
		{Name: "feature"}, {Name: "frontend"}, {Name: "urgent"},
	}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, []string{"feature", "frontend", "urgent"}, remote.Labels)
}
