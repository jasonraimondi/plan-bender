package backend

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// enabledRoot writes a minimal .plan-bender.json with linear enabled to a temp
// dir and returns it, so ensureEnabled's disk re-read passes during tests.
func enabledRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := `{"linear":{"enabled":true,"api_key":"k","team":"team-1"}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".plan-bender.json"), []byte(cfg), 0o644))
	return dir
}

func backendWithCapture(t *testing.T, response string, estimationEnabled bool) (*linearBackend, *captureTransport) {
	t.Helper()
	ct := &captureTransport{response: response}
	client := linear.NewClientWithHTTP(&http.Client{Transport: ct})
	b := &linearBackend{
		client:            client,
		cfg:               config.Defaults(),
		root:              enabledRoot(t),
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

const createIssueResponse = `{"data":{"issueCreate":{"success":true,"issue":{"id":"i1","title":"T","state":{"name":"Backlog"}}}}}`
const updateIssueResponse = `{"data":{"issueUpdate":{"success":true,"issue":{"id":"i1","title":"T","state":{"name":"Backlog"}}}}}`

func TestCreateIssue_EstimationEnabled(t *testing.T) {
	b, ct := backendWithCapture(t, createIssueResponse, true)
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 5}

	_, err := b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.NoError(t, err)
	assert.Contains(t, ct.body, `"estimate":5`)
}

func TestCreateIssue_EstimationDisabled(t *testing.T) {
	b, ct := backendWithCapture(t, createIssueResponse, false)
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 5}

	_, err := b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.NoError(t, err)
	assert.NotContains(t, ct.body, "estimate")
}

func TestUpdateIssue_EstimationEnabled(t *testing.T) {
	b, ct := backendWithCapture(t, updateIssueResponse, true)
	linearID := "lin-1"
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 3, LinearID: &linearID}

	_, err := b.UpdateIssue(t.Context(), issue, "slug")
	require.NoError(t, err)
	assert.Contains(t, ct.body, `"estimate":3`)
}

func TestUpdateIssue_EstimationDisabled(t *testing.T) {
	b, ct := backendWithCapture(t, updateIssueResponse, false)
	linearID := "lin-1"
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog", Points: 3, LinearID: &linearID}

	_, err := b.UpdateIssue(t.Context(), issue, "slug")
	require.NoError(t, err)
	assert.NotContains(t, ct.body, "estimate")
}

func TestEnsureEnabled_RereadsFileOnEachOp(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".plan-bender.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{"linear":{"enabled":true,"api_key":"k","team":"team-1"}}`), 0o644))

	ct := &captureTransport{response: createIssueResponse}
	b := &linearBackend{
		client:   linear.NewClientWithHTTP(&http.Client{Transport: ct}),
		cfg:      config.Defaults(),
		root:     dir,
		teamID:   "team-1",
		stateIDs: map[string]string{"Backlog": "state-1"},
	}
	issue := &schema.Issue{ID: 1, Name: "T", Status: "backlog"}

	// Enabled at op time: the write reaches Linear.
	_, err := b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.NoError(t, err)

	// Disable in the file; the next op must re-read and refuse rather than
	// continue on the stale enabled value.
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{"linear":{"enabled":false}}`), 0o644))

	_, err = b.CreateIssue(t.Context(), issue, "proj-1", "slug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLinearIssueToRemote_MultipleLabels(t *testing.T) {
	issue := &linear.Issue{ID: "lin-3"}
	issue.Labels.Nodes = []struct{ Name string }{
		{Name: "feature"}, {Name: "frontend"}, {Name: "urgent"},
	}

	remote := linearIssueToRemote(issue)
	assert.Equal(t, []string{"feature", "frontend", "urgent"}, remote.Labels)
}
