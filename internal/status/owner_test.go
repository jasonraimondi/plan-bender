package status

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/require"
)

// newTestOwner returns an Owner wired to a fresh in-memory store and a
// captured slog buffer. The buffer makes the "no audit log on no-op"
// assertion possible without touching slog.Default.
func newTestOwner(t *testing.T) (*Owner, *inMemStore, *bytes.Buffer) {
	t.Helper()
	store := newInMemStore()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	o := New(store)
	o.logger = logger
	return o, store, buf
}

func issueAt(id int, status string) schema.IssueYaml {
	return schema.IssueYaml{ID: id, Slug: "x", Name: "x", Status: status}
}

func TestTransition_AllAllowedEdgesSucceed(t *testing.T) {
	cases := []struct {
		from, to Status
	}{
		{StatusTodo, StatusInProgress},
		{StatusTodo, StatusInReview},
		{StatusTodo, StatusBlocked},
		{StatusTodo, StatusCanceled},
		{StatusInProgress, StatusInReview},
		{StatusInProgress, StatusBlocked},
		{StatusInProgress, StatusCanceled},
		{StatusBacklog, StatusInProgress},
		{StatusBacklog, StatusBlocked},
		{StatusBacklog, StatusInReview},
		{StatusInReview, StatusDone},
		{StatusInReview, StatusBlocked},
		{StatusBlocked, StatusTodo},
		{StatusBlocked, StatusInProgress},
	}
	for _, c := range cases {
		t.Run(string(c.from)+"->"+string(c.to), func(t *testing.T) {
			o, store, _ := newTestOwner(t)
			store.seed("p", issueAt(1, string(c.from)))

			err := o.Transition(context.Background(), "p", 1, []Status{c.from}, c.to, "")
			require.NoError(t, err)

			got, err := store.Load("p")
			require.NoError(t, err)
			require.Equal(t, string(c.to), got[0].Status)
		})
	}
}

func TestTransition_DisallowedEdgeReturnsErrIllegalTransition(t *testing.T) {
	// todo → done is not in the allowed table. Set from=[todo] so CAS passes
	// and the allowed-transitions check is what fails.
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusDone, "")

	var illegal *ErrIllegalTransition
	require.ErrorAs(t, err, &illegal)
	require.Equal(t, StatusTodo, illegal.From)
	require.Equal(t, StatusDone, illegal.To)

	got, _ := store.Load("p")
	require.Equal(t, string(StatusTodo), got[0].Status, "illegal transition must not write")
	require.Equal(t, 0, store.saves)
}

func TestTransition_CASMismatchReturnsActualCurrent(t *testing.T) {
	// from=[todo] but issue is in-progress and target is in-review. Current is
	// neither in from-set nor equal to target → ErrCASMismatch.
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusInProgress)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusInReview, "")

	var cas *ErrCASMismatch
	require.ErrorAs(t, err, &cas)
	require.Equal(t, StatusInProgress, cas.Current)
	require.Equal(t, 0, store.saves)
}

func TestTransition_IdempotentNoOpReturnsErrAlreadyInState(t *testing.T) {
	// UC-7: dispatcher restart re-issues Transition(in-review → done) for an
	// already-done issue. Current is NOT in from-set but equals target — must
	// still return ErrAlreadyInState (per PRD CAS rule), no audit log, no write.
	o, store, buf := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusDone)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusInReview}, StatusDone, "ignored reason")

	require.ErrorIs(t, err, ErrAlreadyInState)
	require.Empty(t, buf.String(), "no audit log on idempotent no-op")
	require.Equal(t, 0, store.saves, "no write on idempotent no-op")

	got, _ := store.Load("p")
	require.Equal(t, string(StatusDone), got[0].Status)
	require.Nil(t, got[0].Notes, "no note appended on no-op even when reason is non-empty")
}

func TestTransition_AppendsStructuredNoteOnNonEmptyReason(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusBlocked, "merge conflict in foo.go")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.NotNil(t, got[0].Notes)
	require.Contains(t, *got[0].Notes, "todo→blocked: merge conflict in foo.go")
}

func TestTransition_AppendsToExistingNotesPreservingHistory(t *testing.T) {
	prev := "[2026-01-01 09:00] todo→blocked: earlier failure"
	seed := issueAt(1, string(StatusBlocked))
	seed.Notes = &prev

	o, store, _ := newTestOwner(t)
	store.seed("p", seed)

	err := o.Transition(context.Background(), "p", 1, []Status{StatusBlocked}, StatusTodo, "human retried")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.NotNil(t, got[0].Notes)
	notes := *got[0].Notes
	require.Contains(t, notes, prev, "prior notes preserved")
	require.Contains(t, notes, "blocked→todo: human retried", "new note appended")
}

func TestTransition_NoNoteWhenReasonEmpty(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusInProgress, "")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.Nil(t, got[0].Notes)
}

func TestTransition_AuditLogOnRealTransitionIncludesAllFields(t *testing.T) {
	o, store, buf := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusInReview)))

	err := o.Transition(context.Background(), "p", 1, []Status{StatusInReview}, StatusDone, "merged cleanly")
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "slug=p")
	require.Contains(t, out, "id=1")
	require.Contains(t, out, "from=in-review")
	require.Contains(t, out, "to=done")
	require.Contains(t, out, `reason="merged cleanly"`)
}

func TestTransition_ConcurrentCallsSerializeViaLock(t *testing.T) {
	// Hammer the same plan/issue with N concurrent todo→in-progress calls.
	// With a working plan-wide lock exactly one wins (real transition); the
	// rest observe current=in-progress and return ErrAlreadyInState (idempotent
	// no-op since current == to). Without locking, multiple goroutines could
	// observe current=todo, all pass CAS, and all save — producing more than
	// one Save and >1 success. The 1-success / 1-save assertion proves the
	// load-check-save sequence is serialized.
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	const N = 32
	var wg sync.WaitGroup
	var success, alreadyInState atomic.Int32

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusInProgress, "")
			switch {
			case err == nil:
				success.Add(1)
			case errors.Is(err, ErrAlreadyInState):
				alreadyInState.Add(1)
			}
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, success.Load())
	require.EqualValues(t, N-1, alreadyInState.Load())
	require.Equal(t, 1, store.saves, "exactly one Save reaches the store")

	got, _ := store.Load("p")
	require.Equal(t, string(StatusInProgress), got[0].Status)
}

func TestTransition_UpdatesUpdatedDateOnRealTransition(t *testing.T) {
	o, store, _ := newTestOwner(t)
	seed := issueAt(1, string(StatusTodo))
	seed.Updated = "2020-01-01"
	store.seed("p", seed)

	err := o.Transition(context.Background(), "p", 1, []Status{StatusTodo}, StatusInProgress, "")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.NotEqual(t, "2020-01-01", got[0].Updated, "Updated date must be refreshed on a real transition")
}

func TestTransition_IssueNotFoundIsAnError(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	err := o.Transition(context.Background(), "p", 99, []Status{StatusTodo}, StatusInProgress, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "#99")
}

func TestClaim_FromBacklogSetsBranchAndInProgress(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusBacklog)))

	err := o.Claim(context.Background(), "p", 1, "user/p--1-x", "worktree create")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.Equal(t, string(StatusInProgress), got[0].Status)
	require.NotNil(t, got[0].Branch)
	require.Equal(t, "user/p--1-x", *got[0].Branch)
	require.NotNil(t, got[0].Notes)
	require.Contains(t, *got[0].Notes, "backlog→in-progress: worktree create")
}

func TestClaim_FromTodoSucceeds(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	require.NoError(t, o.Claim(context.Background(), "p", 1, "user/p--1-x", ""))

	got, _ := store.Load("p")
	require.Equal(t, string(StatusInProgress), got[0].Status)
	require.NotNil(t, got[0].Branch)
}

func TestClaim_IdempotentWhenAlreadyInProgressOnSameBranch(t *testing.T) {
	o, store, _ := newTestOwner(t)
	seed := issueAt(1, string(StatusInProgress))
	branch := "user/p--1-x"
	seed.Branch = &branch
	store.seed("p", seed)

	err := o.Claim(context.Background(), "p", 1, branch, "noop")
	require.ErrorIs(t, err, ErrAlreadyInState)
	require.Equal(t, 0, store.saves, "idempotent claim must not write")
}

func TestClaim_RewritesBranchWhenInProgressOnDifferentBranch(t *testing.T) {
	// User force-recreates a worktree; the branch on disk changes. Allow the
	// branch field to be re-stamped while staying in-progress — this is the
	// only path that updates the branch field on an active issue.
	o, store, _ := newTestOwner(t)
	seed := issueAt(1, string(StatusInProgress))
	old := "user/p--1-old"
	seed.Branch = &old
	store.seed("p", seed)

	err := o.Claim(context.Background(), "p", 1, "user/p--1-new", "")
	require.NoError(t, err)

	got, _ := store.Load("p")
	require.NotNil(t, got[0].Branch)
	require.Equal(t, "user/p--1-new", *got[0].Branch)
}

func TestClaim_RefusesWhenInReviewOrDone(t *testing.T) {
	for _, st := range []Status{StatusInReview, StatusDone, StatusCanceled} {
		t.Run(string(st), func(t *testing.T) {
			o, store, _ := newTestOwner(t)
			store.seed("p", issueAt(1, string(st)))

			err := o.Claim(context.Background(), "p", 1, "user/p--1-x", "")
			var cas *ErrCASMismatch
			require.ErrorAs(t, err, &cas)
			require.Equal(t, st, cas.Current)
			require.Equal(t, 0, store.saves)
		})
	}
}

func TestClaim_FromBlockedTransitionsToInProgress(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusBlocked)))

	require.NoError(t, o.Claim(context.Background(), "p", 1, "user/p--1-x", "manual unblock"))

	got, _ := store.Load("p")
	require.Equal(t, string(StatusInProgress), got[0].Status)
}

func TestClaim_RequiresBranch(t *testing.T) {
	o, store, _ := newTestOwner(t)
	store.seed("p", issueAt(1, string(StatusTodo)))

	err := o.Claim(context.Background(), "p", 1, "", "")
	require.Error(t, err)
	require.Equal(t, 0, store.saves)
}
