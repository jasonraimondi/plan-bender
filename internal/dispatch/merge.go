package dispatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// MergeResult reports the outcome of a Merge: the issue IDs whose branches
// merged cleanly into the integration branch (now done) and those whose merge
// conflicted (now blocked, their branch preserved).
type MergeResult struct {
	Merged     []int
	Conflicted []int
}

// Merge integrates completed (in-review) issues into the integration branch in
// dependency order, lifting merge-back out of the dispatch loop so it survives
// as a standalone operation. It merges every in-review issue that has a branch
// set AND whose blocked_by are all done, flipping merged issues to done and
// conflicting ones to blocked (MergeBack does both). All git work happens inside
// the per-slug integration worktree, so the parent repo's HEAD is never touched.
//
// Merge is idempotent: an empty ready set (nothing in-review) is a no-op
// returning a zero MergeResult, and an already-merged branch is a no-op on
// re-entry (MergeBack relies on `git merge --no-ff` reporting "Already up to
// date.").
func (d *Dispatcher) Merge(ctx context.Context, slug string) (MergeResult, error) {
	lockPath := filepath.Join(d.plansDir(), slug, ".dispatch.lock")
	release, err := planrepo.TryFlock(lockPath)
	if err != nil {
		if errors.Is(err, planrepo.ErrLocked) {
			return MergeResult{}, fmt.Errorf("merge already running for slug %q (lock: %s)", slug, lockPath)
		}
		return MergeResult{}, fmt.Errorf("acquiring dispatch lock for slug %q: %w", slug, err)
	}
	defer release()

	issues, err := d.snapshotIssues(ctx, slug)
	if err != nil {
		return MergeResult{}, fmt.Errorf("loading issues: %w", err)
	}

	ready := readyToMerge(issues)
	if len(ready) == 0 {
		return MergeResult{}, nil
	}

	integrationBranch, err := d.ensureIntegrationBranch(ctx, slug)
	if err != nil {
		return MergeResult{}, fmt.Errorf("setting up integration branch: %w", err)
	}

	if err := d.MergeBack(ctx, slug, ready, integrationBranch); err != nil {
		return MergeResult{}, fmt.Errorf("merging in-review issues: %w", err)
	}

	// MergeBack flips merged issues to done and conflicting ones to blocked.
	// Re-read the statuses to classify each candidate: in-review → done is a
	// clean merge; in-review → blocked is a conflict.
	post, err := d.snapshotIssues(ctx, slug)
	if err != nil {
		return MergeResult{}, fmt.Errorf("re-reading issues after merge: %w", err)
	}
	statusByID := make(map[int]string, len(post))
	for _, iss := range post {
		statusByID[iss.ID] = iss.Status
	}

	var result MergeResult
	for _, r := range ready {
		switch statusByID[r.IssueID] {
		case "done":
			result.Merged = append(result.Merged, r.IssueID)
		case "blocked":
			result.Conflicted = append(result.Conflicted, r.IssueID)
		}
	}
	return result, nil
}

// readyToMerge narrows pendingMergeBack's in-review-with-branch set to issues
// whose blocked_by are all satisfied. pendingMergeBack does not check
// dependencies, so without this filter a dependent could merge before its
// blocker's branch landed in integration — violating the acceptance criterion
// that only in-review issues whose blocked_by are all done are merged.
//
// A blocked_by dep is satisfied when it is already done OR it is itself a
// merge candidate in this same batch: MergeBack merges in dependency order
// (successfulInDepOrder), so the blocker's branch lands in integration before
// the dependent's. Filtering is run to a fixpoint so a dependent whose blocker
// was just excluded (its own deps unsatisfied) is excluded too.
func readyToMerge(issues []schema.Issue) []SubResult {
	doneIDs := make(map[int]bool, len(issues))
	for _, iss := range issues {
		if iss.Status == "done" {
			doneIDs[iss.ID] = true
		}
	}
	depsByID := make(map[int][]int, len(issues))
	for _, iss := range issues {
		depsByID[iss.ID] = iss.BlockedBy
	}

	candidates := pendingMergeBack(issues)
	for {
		ready := make(map[int]bool, len(candidates))
		for _, r := range candidates {
			ready[r.IssueID] = true
		}
		kept := candidates[:0]
		for _, r := range candidates {
			if depsSatisfied(depsByID[r.IssueID], doneIDs, ready) {
				kept = append(kept, r)
			}
		}
		if len(kept) == len(candidates) {
			return kept
		}
		candidates = kept
	}
}

func depsSatisfied(blockedBy []int, doneIDs, ready map[int]bool) bool {
	for _, dep := range blockedBy {
		if !doneIDs[dep] && !ready[dep] {
			return false
		}
	}
	return true
}
