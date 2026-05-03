package backend

import (
	"context"
	"testing"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tryOpenWithin attempts to acquire the plan-repo session lock for slug
// and reports whether the acquire returned within timeout. Used to probe
// whether a concurrent code path is already holding the lock — POSIX flock
// is exclusive even between two fds in the same process, so the second
// attempt blocks while another holder is alive.
func tryOpenWithin(plans *planrepo.Plans, slug string, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		sess, err := plans.Open(slug)
		if err == nil {
			_ = sess.Close()
		}
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestSyncPush_NoLockDuringRemoteCreateIssue exercises acceptance criterion
// "Sync never holds a plan repository session lock while calling Linear or
// other remote Backend APIs": the remote stub probes the plan lock from a
// goroutine; if SyncPush is still holding the snapshot session, the probe
// would block and the assertion fails. The probe runs once during the first
// CreateIssue call so we exercise the per-issue iteration path, not just the
// initial readSnapshot release.
func TestSyncPush_NoLockDuringRemoteCreateIssue(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}
	fix := setupSyncTest(t, prd, []*schema.IssueYaml{testIssue(1)})

	probed := false
	probeOK := false
	be := &mockBackend{
		createIssue: func(_ context.Context, issue *schema.IssueYaml, _ string) (RemoteIssue, error) {
			if !probed {
				probed = true
				probeOK = tryOpenWithin(fix.plans, "test", 500*time.Millisecond)
			}
			return RemoteIssue{ID: "lin-1"}, nil
		},
	}

	result, err := SyncPush(context.Background(), fix.plans, be, "test", fix.cfg)
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	assert.True(t, probed, "createIssue stub must be invoked")
	assert.True(t, probeOK, "plan lock must be released before remote CreateIssue runs")
}

// TestSyncPull_NoLockDuringRemotePullProject exercises the same invariant
// for the pull path: PullProject is the only remote call SyncPull makes,
// and it must run after the snapshot session has released the lock.
func TestSyncPull_NoLockDuringRemotePullProject(t *testing.T) {
	prd := testPrd()
	prd.Linear = &schema.LinearRef{ProjectID: "proj-1"}
	linID := "lin-1"
	issue := testIssue(1)
	issue.LinearID = &linID
	fix := setupSyncTest(t, prd, []*schema.IssueYaml{issue})

	probeOK := false
	be := &mockBackend{
		pullProject: func(_ context.Context, _ string) (PullProjectResult, error) {
			probeOK = tryOpenWithin(fix.plans, "test", 500*time.Millisecond)
			return PullProjectResult{
				Project: RemoteProject{ID: "proj-1"},
				Issues:  []RemoteIssue{{ID: "lin-1", Status: "in-progress"}},
			}, nil
		},
	}

	_, err := SyncPull(context.Background(), fix.plans, be, "test", fix.cfg)
	require.NoError(t, err)
	assert.True(t, probeOK, "plan lock must be released before remote PullProject runs")
}

// TestSyncPush_NoLockDuringRemoteCreateProject covers the third remote entry
// point: CreateProject (called when the PRD has no linear.project_id).
func TestSyncPush_NoLockDuringRemoteCreateProject(t *testing.T) {
	prd := testPrd() // no Linear ref → triggers CreateProject
	fix := setupSyncTest(t, prd, nil)

	probeOK := false
	be := &mockBackend{
		createProject: func(_ context.Context, _ *schema.PrdYaml) (RemoteProject, error) {
			probeOK = tryOpenWithin(fix.plans, "test", 500*time.Millisecond)
			return RemoteProject{ID: "new-proj"}, nil
		},
	}

	_, err := SyncPush(context.Background(), fix.plans, be, "test", fix.cfg)
	require.NoError(t, err)
	assert.True(t, probeOK, "plan lock must be released before remote CreateProject runs")
}
