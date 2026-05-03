package backend

import (
	"context"
	"errors"
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// SyncError records a per-issue failure during sync.
type SyncError struct {
	IssueID int
	Err     error
}

func (e SyncError) Error() string {
	return fmt.Sprintf("issue #%d: %v", e.IssueID, e.Err)
}

func (e SyncError) Unwrap() error {
	return e.Err
}

// SyncResult reports the outcome of a push or pull operation.
type SyncResult struct {
	Created int
	Updated int
	Errors  []SyncError
}

func joinSyncErrors(errs []SyncError) error {
	joined := make([]error, 0, len(errs))
	for _, err := range errs {
		joined = append(joined, err)
	}
	return errors.Join(joined...)
}

// SyncPush pushes local issues to the remote backend.
//
// To honor the "no plan lock held during remote API calls" invariant the
// flow is split into discrete phases:
//
//  1. Open a session, copy the snapshot, Close — releases the lock before
//     any remote work begins.
//  2. Ensure project exists; if a CreateProject API call is required, write
//     the returned project ID back through a fresh short session.
//  3. For each issue, perform the remote create/update with no lock held,
//     then write the linear_id back (on create) through a per-issue short
//     session so a single write failure does not block the rest.
func SyncPush(ctx context.Context, plans *planrepo.Plans, be Backend, slug string, cfg config.Config) (SyncResult, error) {
	var result SyncResult

	prd, issues, err := readSnapshot(plans, slug)
	if err != nil {
		return result, err
	}

	projectID, err := ensureRemoteProject(ctx, plans, be, slug, &prd, cfg)
	if err != nil {
		return result, err
	}

	remoteAttempts := 0
	remoteFailures := 0
	for i := range issues {
		issue := &issues[i]
		if issue.LinearID != nil && *issue.LinearID != "" {
			remoteAttempts++
			if _, err := be.UpdateIssue(ctx, issue); err != nil {
				remoteFailures++
				result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
				continue
			}
			result.Updated++
			continue
		}
		remoteAttempts++
		remote, err := be.CreateIssue(ctx, issue, projectID)
		if err != nil {
			remoteFailures++
			result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
			continue
		}
		remoteID := remote.ID
		issue.LinearID = &remoteID
		if err := commitIssue(plans, slug, *issue, cfg); err != nil {
			result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
			continue
		}
		result.Created++
	}

	if remoteAttempts > 0 && remoteFailures == remoteAttempts {
		return result, fmt.Errorf("sync push failed for all %d remote issue attempts: %w", remoteAttempts, joinSyncErrors(result.Errors))
	}

	return result, nil
}

// SyncPull pulls remote state and updates local issues.
//
// Same lock-discipline as SyncPush: read snapshot under a short session,
// release before the remote PullProject call, then write each dirty issue
// back through a per-issue short session.
func SyncPull(ctx context.Context, plans *planrepo.Plans, be Backend, slug string, cfg config.Config) (SyncResult, error) {
	var result SyncResult

	prd, issues, err := readSnapshot(plans, slug)
	if err != nil {
		return result, err
	}
	if prd.Linear == nil || prd.Linear.ProjectID == "" {
		return result, fmt.Errorf("no linear.project_id in PRD — run push first")
	}

	remote, err := be.PullProject(ctx, prd.Linear.ProjectID)
	if err != nil {
		return result, fmt.Errorf("pulling project: %w", err)
	}

	remoteByID := make(map[string]RemoteIssue, len(remote.Issues))
	for _, ri := range remote.Issues {
		remoteByID[ri.ID] = ri
	}

	for i := range issues {
		issue := &issues[i]
		if issue.LinearID == nil || *issue.LinearID == "" {
			continue
		}
		ri, ok := remoteByID[*issue.LinearID]
		if !ok {
			continue
		}
		if !applyRemoteFields(issue, ri) {
			continue
		}
		if err := commitIssue(plans, slug, *issue, cfg); err != nil {
			result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
			continue
		}
		result.Updated++
	}

	return result, nil
}

// readSnapshot opens a short session, returns copies of the PRD and issues,
// then closes — guaranteeing the plan lock is no longer held when remote
// API calls begin.
func readSnapshot(plans *planrepo.Plans, slug string) (schema.PrdYaml, []schema.IssueYaml, error) {
	sess, err := plans.Open(slug)
	if err != nil {
		return schema.PrdYaml{}, nil, err
	}
	defer sess.Close()
	snap, err := sess.Snapshot()
	if err != nil {
		return schema.PrdYaml{}, nil, err
	}
	return snap.PRD, snap.Issues, nil
}

// ensureRemoteProject returns the projectID to push issues against. When the
// PRD already has a Linear ProjectID it is used as-is; otherwise CreateProject
// is called (no lock held) and the returned ID is written back to the PRD via
// a fresh session.
func ensureRemoteProject(ctx context.Context, plans *planrepo.Plans, be Backend, slug string, prd *schema.PrdYaml, cfg config.Config) (string, error) {
	if prd.Linear != nil && prd.Linear.ProjectID != "" {
		return prd.Linear.ProjectID, nil
	}
	project, err := be.CreateProject(ctx, prd)
	if err != nil {
		return "", fmt.Errorf("creating project: %w", err)
	}
	if prd.Linear == nil {
		prd.Linear = &schema.LinearRef{}
	}
	prd.Linear.ProjectID = project.ID
	if err := commitPRD(plans, slug, *prd, cfg); err != nil {
		return "", fmt.Errorf("writing PRD: %w", err)
	}
	return project.ID, nil
}

// applyRemoteFields mirrors a remote issue's mutable fields into the local
// copy, returning true when anything changed (i.e. a write-back is needed).
func applyRemoteFields(local *schema.IssueYaml, remote RemoteIssue) bool {
	dirty := false
	if remote.Status != "" && remote.Status != local.Status {
		local.Status = remote.Status
		dirty = true
	}
	if remote.Priority != "" && remote.Priority != local.Priority {
		local.Priority = remote.Priority
		dirty = true
	}
	if remote.Assignee != "" {
		if local.Assignee == nil || *local.Assignee != remote.Assignee {
			a := remote.Assignee
			local.Assignee = &a
			dirty = true
		}
	}
	return dirty
}

func commitPRD(plans *planrepo.Plans, slug string, prd schema.PrdYaml, cfg config.Config) error {
	sess, err := plans.Open(slug)
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := sess.UpdatePrd(prd); err != nil {
		return err
	}
	return sess.Commit(cfg)
}

// commitIssue opens a fresh session per call so the snapshot reflects any
// writes a concurrent process made between the original readSnapshot and
// this commit. UpdateIssue may surface "id not in session snapshot" if the
// issue was deleted out from under us; the caller turns that into a
// per-issue SyncError so one disappearance doesn't abort the whole run.
func commitIssue(plans *planrepo.Plans, slug string, issue schema.IssueYaml, cfg config.Config) error {
	sess, err := plans.Open(slug)
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := sess.UpdateIssue(issue); err != nil {
		return err
	}
	return sess.Commit(cfg)
}
