package backend

import (
	"context"
	"fmt"

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

// SyncResult reports the outcome of a push or pull operation.
type SyncResult struct {
	Created int
	Updated int
	Errors  []SyncError
}

// SyncPush pushes local issues to the remote backend.
// It creates or updates each issue, writes linear_id back on success,
// and continues on per-issue errors.
func SyncPush(ctx context.Context, store *PlanStore, be Backend, slug string) (SyncResult, error) {
	var result SyncResult

	// Read PRD
	prd, err := store.ReadPrd(slug)
	if err != nil {
		return result, err
	}

	// Ensure project exists
	projectID := ""
	if prd.Linear != nil && prd.Linear.ProjectID != "" {
		projectID = prd.Linear.ProjectID
	} else {
		project, err := be.CreateProject(ctx, prd)
		if err != nil {
			return result, fmt.Errorf("creating project: %w", err)
		}
		projectID = project.ID
		if prd.Linear == nil {
			prd.Linear = &schema.LinearRef{}
		}
		prd.Linear.ProjectID = projectID
		if err := store.WritePrd(slug, prd); err != nil {
			return result, fmt.Errorf("writing PRD: %w", err)
		}
	}

	// Read and push issues
	issues, err := store.ReadIssues(slug)
	if err != nil {
		return result, err
	}

	for i := range issues {
		issue := &issues[i]
		if issue.LinearID != nil && *issue.LinearID != "" {
			// Update existing
			if _, err := be.UpdateIssue(ctx, issue); err != nil {
				result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
				continue
			}
			result.Updated++
		} else {
			// Create new
			remote, err := be.CreateIssue(ctx, issue, projectID)
			if err != nil {
				result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
				continue
			}
			// Write linear_id back per-issue
			remoteID := remote.ID
			issue.LinearID = &remoteID
			if err := store.WriteIssue(slug, issue); err != nil {
				result.Errors = append(result.Errors, SyncError{IssueID: issue.ID, Err: err})
				continue
			}
			result.Created++
		}
	}

	return result, nil
}
