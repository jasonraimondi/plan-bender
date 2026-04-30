package plan

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr returns a pointer to s. Used to set assignee on test fixtures.
func strPtr(s string) *string { return &s }

// issueOpt mutates a fixture issue. Tests compose them to keep rows readable.
type issueOpt func(*schema.IssueYaml)

func mkIssue(id int, status, priority string, opts ...issueOpt) schema.IssueYaml {
	iss := schema.IssueYaml{
		ID:       id,
		Slug:     "issue",
		Name:     "issue",
		Status:   status,
		Priority: priority,
		Labels:   []string{"AFK"},
	}
	for _, opt := range opts {
		opt(&iss)
	}
	return iss
}

func withLabels(l ...string) issueOpt {
	return func(i *schema.IssueYaml) { i.Labels = l }
}

func withBlockedBy(ids ...int) issueOpt {
	return func(i *schema.IssueYaml) { i.BlockedBy = ids }
}

func withAssignee(name string) issueOpt {
	return func(i *schema.IssueYaml) { i.Assignee = strPtr(name) }
}

func TestResolve_InProgressPreemptsAll(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "urgent"),
		mkIssue(2, "in-progress", "low"),
		mkIssue(3, "backlog", "urgent"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
}

func TestResolve_TodoBeatsBacklog(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "backlog", "urgent"),
		mkIssue(2, "todo", "low"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
}

func TestResolve_AFKBeatsHITL(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "urgent", withLabels("HITL")),
		mkIssue(2, "todo", "low", withLabels("AFK")),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
	assert.False(t, r.RequiresHuman)
}

func TestResolve_HITLSurfacesWhenNoAFK(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "urgent", withLabels("HITL")),
		mkIssue(2, "todo", "high", withLabels("HITL")),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 1, r.Issue.ID)
	assert.True(t, r.RequiresHuman)
}

func TestResolve_PriorityWithinStatusTier(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "low"),
		mkIssue(2, "todo", "urgent"),
		mkIssue(3, "todo", "medium"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
}

func TestResolve_TieBreakByUnblocksCount(t *testing.T) {
	// Two todo+high candidates: id=1 unblocks 2 issues, id=2 unblocks 0.
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "high"),
		mkIssue(2, "todo", "high"),
		mkIssue(3, "backlog", "low", withBlockedBy(1)),
		mkIssue(4, "backlog", "low", withBlockedBy(1)),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 1, r.Issue.ID)
}

func TestResolve_FinalTieBreakByID(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(2, "todo", "high"),
		mkIssue(1, "todo", "high"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 1, r.Issue.ID)
}

func TestResolve_StaleBlockedPromoted(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "done", "high"),
		mkIssue(2, "blocked", "high", withBlockedBy(1)),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
	assert.True(t, r.WasBlocked)
}

func TestResolve_BlockedByOpenIsNotReady(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "high"),
		mkIssue(2, "todo", "urgent", withBlockedBy(1)),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 1, r.Issue.ID)
}

func TestResolve_AssignedSkipped(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "urgent", withAssignee("alice")),
		mkIssue(2, "todo", "low"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
	require.Len(t, r.Skipped, 1)
	assert.Equal(t, 1, r.Skipped[0].ID)
	assert.Contains(t, r.Skipped[0].Reason, "assigned")
}

func TestResolve_AllDoneEmptyResult(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "done", "high"),
		mkIssue(2, "canceled", "high"),
	}
	r := Resolve(issues)
	assert.Nil(t, r.Issue)
	assert.True(t, r.AllDone)
	assert.Equal(t, 0, r.BlockedCount)
}

func TestResolve_NoCandidatesNotAllDone(t *testing.T) {
	// One blocked by an open issue that itself is assigned.
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "high", withAssignee("alice")),
		mkIssue(2, "todo", "high", withBlockedBy(1)),
	}
	r := Resolve(issues)
	assert.Nil(t, r.Issue)
	assert.False(t, r.AllDone)
}

func TestResolve_BlockedCountReflectsTrueBlocked(t *testing.T) {
	// One stale-blocked (ready), one truly blocked (deps not all done).
	issues := []schema.IssueYaml{
		mkIssue(1, "done", "high"),
		mkIssue(2, "todo", "high"),
		mkIssue(3, "blocked", "high", withBlockedBy(1)), // stale, ready
		mkIssue(4, "blocked", "high", withBlockedBy(2)), // truly blocked
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	// id=2 (todo, high) wins because status:todo outranks stale-blocked
	assert.Equal(t, 2, r.Issue.ID)
	assert.Equal(t, 1, r.BlockedCount, "only id=4 is truly blocked")
}

func TestResolve_StatusOrderInProgressTodoBacklogStaleBlocked(t *testing.T) {
	// One of each, all ready (stale-blocked depends on done).
	issues := []schema.IssueYaml{
		mkIssue(1, "done", "high"),
		mkIssue(2, "blocked", "urgent", withBlockedBy(1)),
		mkIssue(3, "backlog", "urgent"),
		mkIssue(4, "todo", "low"),
		mkIssue(5, "in-progress", "low"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 5, r.Issue.ID, "in-progress wins")

	// Drop in-progress: todo wins
	r = Resolve(issues[:4])
	require.NotNil(t, r.Issue)
	assert.Equal(t, 4, r.Issue.ID)

	// Drop todo: backlog wins
	r = Resolve(issues[:3])
	require.NotNil(t, r.Issue)
	assert.Equal(t, 3, r.Issue.ID)

	// Drop backlog: stale-blocked wins
	r = Resolve(issues[:2])
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)
	assert.True(t, r.WasBlocked)
}

func TestResolve_SkippedListsEverythingNotChosen(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "done", "high"),
		mkIssue(2, "todo", "high"),
		mkIssue(3, "todo", "low"),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)

	skippedByID := make(map[int]string, len(r.Skipped))
	for _, s := range r.Skipped {
		skippedByID[s.ID] = s.Reason
	}
	assert.Contains(t, skippedByID, 1)
	assert.Contains(t, skippedByID, 3)
	assert.NotContains(t, skippedByID, 2)
}

func TestResolve_HITLSkippedReasonWhenAFKWins(t *testing.T) {
	issues := []schema.IssueYaml{
		mkIssue(1, "todo", "urgent", withLabels("HITL")),
		mkIssue(2, "todo", "low", withLabels("AFK")),
	}
	r := Resolve(issues)
	require.NotNil(t, r.Issue)
	assert.Equal(t, 2, r.Issue.ID)

	var hitlReason string
	for _, s := range r.Skipped {
		if s.ID == 1 {
			hitlReason = s.Reason
		}
	}
	assert.Contains(t, hitlReason, "HITL")
}
