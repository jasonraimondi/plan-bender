package status

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Owner mediates every issue-status write. Callers go through Transition;
// they never see the plan lock or the underlying store.
type Owner struct {
	store  Store
	logger *slog.Logger
}

// New returns an Owner backed by store and using the package-default slog
// logger. Tests in this package may override the logger field directly.
func New(store Store) *Owner {
	return &Owner{store: store, logger: slog.Default()}
}

// Transition atomically advances issue id from a state in `from` to `to`.
//
// Algorithm (per PRD decision "current must be in from-set OR equal to target"):
//
//  1. Open a store session (which holds the plan lock).
//  2. CAS check: current must be in `from` OR equal to `to`. Otherwise
//     return *ErrCASMismatch carrying the actual current state.
//  3. Idempotency: if current == to, return ErrAlreadyInState — no audit
//     log, no write. This covers UC-7 (dispatcher restart re-issues a
//     transition that already landed).
//  4. Allowed-transitions check: (current → to) must be in the hardcoded
//     allowed table. Otherwise return *ErrIllegalTransition.
//  5. Save the mutated issue through the session, append a structured note
//     when reason is non-empty, update the Updated date, and emit a single
//     slog.Info audit line.
//
// The ctx is currently used only for cancellation symmetry with future
// callers; the backend store does not yet take a ctx.
func (o *Owner) Transition(ctx context.Context, slug string, id int, from []Status, to Status, reason string) error {
	_ = ctx

	sess, err := o.store.OpenSession(slug)
	if err != nil {
		return fmt.Errorf("opening session for plan %q: %w", slug, err)
	}
	defer func() {
		if cerr := sess.Close(); cerr != nil {
			o.logger.Warn("status: session close failed", "slug", slug, "err", cerr)
		}
	}()

	issues := sess.Issues()
	idx := -1
	for i := range issues {
		if issues[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("issue #%d not found in plan %q", id, slug)
	}

	current := Status(issues[idx].Status)

	if !containsStatus(from, current) && current != to {
		return &ErrCASMismatch{Current: current}
	}
	if current == to {
		return ErrAlreadyInState
	}
	if !isAllowed(current, to) {
		return &ErrIllegalTransition{From: current, To: to}
	}

	issue := issues[idx]
	issue.Status = string(to)
	issue.Updated = time.Now().Format("2006-01-02")
	if reason != "" {
		appendNote(&issue, current, to, reason)
	}
	if err := sess.Save(issue); err != nil {
		return fmt.Errorf("saving issue #%d: %w", id, err)
	}

	o.logger.Info("status: transition",
		"slug", slug,
		"id", id,
		"from", string(current),
		"to", string(to),
		"reason", reason,
	)
	return nil
}

// Claim atomically marks an issue as in-progress and records the branch a
// worker has reserved for it. Used by `worktree create` so the YAML truthfully
// reflects on-disk state after a worktree is created — without Claim the
// branch lives on disk, but the issue YAML still says backlog/todo with no
// branch field, and dispatch's CAS-checks lose the thread.
//
// Claim is idempotent: if the issue is already in-progress on the same
// branch, Claim returns ErrAlreadyInState (no write). If it is in-progress
// on a different branch, Claim returns *ErrCASMismatch with that branch's
// state instead of silently overwriting work.
//
// CAS from-set: [backlog, todo, blocked, in-progress]. Anything else (e.g.
// in-review, done, canceled) is refused via *ErrCASMismatch — claiming a
// completed issue is almost certainly a mistake worth surfacing.
func (o *Owner) Claim(ctx context.Context, slug string, id int, branch, reason string) error {
	_ = ctx

	if branch == "" {
		return fmt.Errorf("claim: branch is required")
	}

	sess, err := o.store.OpenSession(slug)
	if err != nil {
		return fmt.Errorf("opening session for plan %q: %w", slug, err)
	}
	defer func() {
		if cerr := sess.Close(); cerr != nil {
			o.logger.Warn("status: session close failed", "slug", slug, "err", cerr)
		}
	}()

	issues := sess.Issues()
	idx := -1
	for i := range issues {
		if issues[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("issue #%d not found in plan %q", id, slug)
	}

	current := Status(issues[idx].Status)
	from := []Status{StatusBacklog, StatusTodo, StatusBlocked, StatusInProgress}
	if !containsStatus(from, current) {
		return &ErrCASMismatch{Current: current}
	}

	issue := issues[idx]
	branchAlreadySet := issue.Branch != nil && *issue.Branch == branch
	if current == StatusInProgress && branchAlreadySet {
		return ErrAlreadyInState
	}

	if current != StatusInProgress && !isAllowed(current, StatusInProgress) {
		return &ErrIllegalTransition{From: current, To: StatusInProgress}
	}

	branchCopy := branch
	issue.Branch = &branchCopy
	issue.Status = string(StatusInProgress)
	issue.Updated = time.Now().Format("2006-01-02")
	if reason != "" && current != StatusInProgress {
		appendNote(&issue, current, StatusInProgress, reason)
	}
	if err := sess.Save(issue); err != nil {
		return fmt.Errorf("saving issue #%d: %w", id, err)
	}

	o.logger.Info("status: claim",
		"slug", slug,
		"id", id,
		"from", string(current),
		"to", string(StatusInProgress),
		"branch", branch,
		"reason", reason,
	)
	return nil
}

func containsStatus(set []Status, s Status) bool {
	for _, x := range set {
		if x == s {
			return true
		}
	}
	return false
}

// appendNote appends a single-line structured note in the format
//
//	[YYYY-MM-DD HH:MM] from→to: reason
//
// preserving any pre-existing notes with a single newline separator. The
// single-line shape is required so SyncPull and template rendering tolerate
// repeated transitions piling notes onto the same issue.
func appendNote(issue *schema.IssueYaml, from, to Status, reason string) {
	line := fmt.Sprintf("[%s] %s→%s: %s", time.Now().Format("2006-01-02 15:04"), from, to, reason)
	if issue.Notes == nil || *issue.Notes == "" {
		issue.Notes = &line
		return
	}
	merged := *issue.Notes + "\n" + line
	issue.Notes = &merged
}
