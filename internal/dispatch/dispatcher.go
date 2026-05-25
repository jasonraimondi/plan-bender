package dispatch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
	"github.com/jasonraimondi/plan-bender/internal/worktree"
)

// ErrHITLOnly signals only HITL issues remain. The CLI maps this to exit code 2.
var ErrHITLOnly = errors.New("only HITL issues remain")

// ErrSetupFailed marks the dispatch failure where every ready issue failed
// environment setup before any sub-agent ran. The CLI keys the report_bugs
// artifact off this: unlike stuck-on-blocked or lock contention (both
// user-resolvable, not bugs), a setup failure is the one exit-1 shape the
// agent-facing report_bugs prompt can't cover, because no sub-agent runs.
var ErrSetupFailed = errors.New("dispatch setup failed")

// Dispatcher orchestrates the full implementation loop for a plan: resolve →
// worktrees → spawn claude subprocesses → merge → cleanup, repeating until
// all_done or HITL-only.
type Dispatcher struct {
	Config config.Config
	Root   string // absolute path to the parent repo

	// PlansDir overrides Config.PlansDir when set; mainly for tests.
	PlansDir string

	// Base overrides the auto-detected default branch as the fork point for
	// the integration branch (or the merge target under `direct` strategy).
	// Empty preserves the auto-detect path. Validated by the CLI layer before
	// the Dispatcher runs.
	Base string

	// Out is where prefixed sub-agent stdout is streamed. Defaults to os.Stdout.
	Out io.Writer

	// gitMu serializes git plumbing operations on Root. Concurrent
	// `git worktree add` invocations deadlock on git's internal locks.
	gitMu sync.Mutex

	// outOnce + outWriter memoize the synchronized writer wrapping d.Out so
	// every goroutine streaming sub-agent output shares one mutex.
	outOnce   sync.Once
	outWriter io.Writer

	// ownerOnce + owner memoize the status.Owner so every status write in a
	// Run goes through one lock-aware adapter without re-allocating. The Owner
	// wraps its own planrepo.Plans handle (NewProdStatusOwner), distinct from
	// `plans` below but rooted at the same plansDir.
	ownerOnce sync.Once
	owner     *status.Owner

	// plansOnce + plans memoize the planrepo.Plans handle for every read in a
	// Run: the resolver and merge-order snapshots, plus the post-subprocess
	// loadIssue read passed into RunSubprocess. It does not back status writes
	// — those go through the Owner's own handle (see ownerOnce) — but all
	// handles target the same plansDir, the single on-disk persistence boundary.
	plansOnce sync.Once
	plans     *planrepo.Plans
}

// plansRepo returns the lazily-constructed planrepo.Plans handle rooted at
// d.plansDir(). All read snapshots inside a Run flow through this handle.
func (d *Dispatcher) plansRepo() *planrepo.Plans {
	d.plansOnce.Do(func() {
		d.plans = planrepo.NewProd(d.plansDir())
	})
	return d.plans
}

// snapshotIssues opens a short-lived planrepo session, reads the issue list,
// and closes the session before returning. The lock is released before the
// caller proceeds so subsequent status writes (or batch goroutines) can take
// the same lock without deadlocking. ctx governs lock acquisition only — a
// canceled ctx unblocks a wait for a contended plan lock instead of holding
// the dispatch loop hostage.
func (d *Dispatcher) snapshotIssues(ctx context.Context, slug string) ([]schema.Issue, error) {
	return snapshotPlanIssues(ctx, d.plansRepo(), slug)
}

func snapshotPlanIssues(ctx context.Context, plans *planrepo.Plans, slug string) ([]schema.Issue, error) {
	sess, err := plans.OpenContext(ctx, slug)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	issues := sess.Snapshot().Issues
	cp := make([]schema.Issue, len(issues))
	copy(cp, issues)
	return cp, nil
}

// statusOwner returns the lazily-constructed status.Owner backed by the
// production planrepo status adapter wired to d.plansDir(). All status writes
// during a Run flow through this single Owner.
func (d *Dispatcher) statusOwner() *status.Owner {
	d.ownerOnce.Do(func() {
		d.owner = planrepo.NewProdStatusOwner(d.plansDir(), d.Config)
	})
	return d.owner
}

// lockedWriter serializes Write calls so concurrent goroutines streaming
// sub-agent stdout don't interleave at the byte level. POSIX guarantees
// write() atomicity only up to PIPE_BUF (4KB), well below stream-json line
// sizes that embed tool outputs.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

func (d *Dispatcher) plansDir() string {
	if d.PlansDir != "" {
		return d.PlansDir
	}
	if filepath.IsAbs(d.Config.PlansDir) {
		return d.Config.PlansDir
	}
	return filepath.Join(d.Root, d.Config.PlansDir)
}

func (d *Dispatcher) out() io.Writer {
	d.outOnce.Do(func() {
		base := d.Out
		if base == nil {
			base = os.Stdout
		}
		d.outWriter = &lockedWriter{w: base}
	})
	return d.outWriter
}

func (d *Dispatcher) strategy() string {
	s := d.Config.Pipeline.BranchStrategy
	if s == "" {
		return "integration"
	}
	return s
}

// Run executes the full dispatch loop until all_done or HITL-only.
// Returns ErrHITLOnly when only human-input issues remain.
func (d *Dispatcher) Run(ctx context.Context, slug string) error {
	lockPath := filepath.Join(d.plansDir(), slug, ".dispatch.lock")
	release, err := planrepo.TryFlock(lockPath)
	if err != nil {
		if errors.Is(err, planrepo.ErrLocked) {
			return fmt.Errorf("dispatch already running for slug %q (lock: %s)", slug, lockPath)
		}
		return fmt.Errorf("acquiring dispatch lock for slug %q: %w", slug, err)
	}
	defer release()

	integrationBranch, err := d.ensureIntegrationBranch(ctx, slug)
	if err != nil {
		return fmt.Errorf("setting up integration branch: %w", err)
	}

	for {
		issues, err := d.snapshotIssues(ctx, slug)
		if err != nil {
			return fmt.Errorf("loading issues: %w", err)
		}

		// Recover from a previous run that crashed between the sub-agent
		// completing (status flipped to in-review by `pba complete`) and
		// MergeBack running. Without this, ReadyAFK skips the in-review
		// issue, openBlockers keeps its dependents unready, and Run hits
		// the "stuck; 0 blocked" error path with no actionable signal.
		if recovery := pendingMergeBack(issues); len(recovery) > 0 {
			if err := d.MergeBack(ctx, slug, recovery, integrationBranch); err != nil {
				return fmt.Errorf("recovering in-review issues: %w", err)
			}
			continue
		}

		res := plan.Resolve(issues)
		if res.AllDone {
			// Final cleanup: remove the per-slug integration worktree along with
			// any remaining issue worktrees. Runs from d.Root (not the iwt) so
			// `git worktree remove` can target the iwt itself, and `branch -d`
			// resolves reachability against the parent's HEAD — an unmerged
			// integration branch is preserved with a warning rather than dropped.
			if _, err := worktree.GC(ctx, d.Root, slug, nil, d.out(), true); err != nil {
				return fmt.Errorf("final worktree gc: %w", err)
			}
			return nil
		}

		batch := plan.ReadyAFK(issues)
		if len(batch) == 0 {
			if hitlOnlyRemaining(issues) {
				d.printHITLSummary(issues)
				return ErrHITLOnly
			}
			return fmt.Errorf("dispatch stuck: no AFK candidates ready and no HITL issues; %s", blockedSummary(issues))
		}

		results := d.RunBatch(ctx, slug, batch, integrationBranch)

		// When every ready issue failed during setup (no sub-agent ran), the
		// next loop would find them all blocked and report "stuck: N blocked",
		// which reads like a dependency deadlock. Surface the real cause once.
		if cause, ok := allSetupFailed(results); ok {
			return fmt.Errorf("%w for every ready issue (%s); this is an environment problem, not a dependency deadlock — see the bender-implement-prd troubleshooting note on worktree skill-linking", ErrSetupFailed, cause)
		}

		if err := d.MergeBack(ctx, slug, results, integrationBranch); err != nil {
			return fmt.Errorf("merging batch: %w", err)
		}
	}
}

// RunBatch dispatches issues through a worker pool capped at
// ResolvedMaxParallel(): at most that many claude subprocesses run
// concurrently. Each worker creates a worktree off integrationBranch, renders
// a prompt, and runs a claude subprocess. Results are returned in input order.
func (d *Dispatcher) RunBatch(ctx context.Context, slug string, issues []schema.Issue, integrationBranch string) []SubResult {
	logDir := filepath.Join(d.Root, ".plan-bender", "logs", slug)

	results := make([]SubResult, len(issues))
	sem := make(chan struct{}, d.Config.Pipeline.ResolvedMaxParallel())
	var wg sync.WaitGroup

	for i := range issues {
		wg.Add(1)
		go func(idx int, issue schema.Issue) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = d.runOne(ctx, slug, issue, logDir, integrationBranch)
		}(i, issues[i])
	}

	wg.Wait()
	return results
}

func (d *Dispatcher) runOne(ctx context.Context, slug string, issue schema.Issue, logDir, integrationBranch string) SubResult {
	d.gitMu.Lock()
	wt, err := worktree.Create(ctx, d.Config, d.Root, slug, issue.ID, issue.Slug, integrationBranch)
	d.gitMu.Unlock()
	if err != nil {
		reason := fmt.Sprintf("creating worktree: %v", err)
		d.markBlockedAndWarn(slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Err: newSetupError(reason, "")}
	}

	// Atomic claim: stamp branch + flip to in-progress through the canonical
	// struct round-trip path. Without this the sub-agent's prompt still shows
	// status: backlog/todo with branch: null, and the implement-issue skill
	// instructs it to "set branch" by textual edit — Edit on a non-unique
	// substring or a naive append produces duplicate `branch` keys, which the
	// strict JSON decoder then rejects on every subsequent Load.
	if err := d.statusOwner().Claim(ctx, slug, issue.ID, wt.Branch, "dispatch worktree"); err != nil && !errors.Is(err, status.ErrAlreadyInState) {
		reason := fmt.Sprintf("claiming issue: %v", err)
		d.markBlockedAndWarn(slug, issue.ID, reason)
		d.cleanupWorktree(wt.Path)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: newSetupError(reason, wt.Path)}
	}
	// Mirror the on-disk update into the in-memory copy so BuildPrompt embeds
	// the post-claim state. Otherwise the sub-agent's prompt shows backlog/null
	// and the skill body talks it into re-stamping the same fields by hand.
	issue.Status = string(status.StatusInProgress)
	branchCopy := wt.Branch
	issue.Branch = &branchCopy

	if err := linkPlansDir(d.Root, wt.Path); err != nil {
		reason := fmt.Sprintf("linking plans dir: %v", err)
		d.markBlockedAndWarn(slug, issue.ID, reason)
		d.cleanupWorktree(wt.Path)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: newSetupError(reason, wt.Path)}
	}

	if hook := d.Config.Hooks.BeforeIssue; hook != "" {
		if stderr, err := RunHook(ctx, hook, wt.Path, d.out()); err != nil {
			reason := fmt.Sprintf("before_issue hook failed: %v\n%s", err, stderr)
			d.markBlockedAndWarn(slug, issue.ID, reason)
			d.cleanupWorktree(wt.Path)
			return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: newSetupError(reason, wt.Path)}
		}
	}

	prompt, err := BuildPrompt(wt.Path, issue)
	if err != nil {
		reason := fmt.Sprintf("building prompt: %v", err)
		d.markBlockedAndWarn(slug, issue.ID, reason)
		d.cleanupWorktree(wt.Path)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: newSetupError(reason, wt.Path)}
	}

	subCtx, cancel := context.WithTimeout(ctx, d.Config.Pipeline.ResolvedSubprocessTimeout())
	defer cancel()
	res := RunSubprocess(subCtx, d.statusOwner(), d.plansRepo(), slug, issue, prompt, wt.Path, logDir, d.out())
	res.Branch = wt.Branch

	if hook := d.Config.Hooks.AfterIssue; hook != "" {
		if _, err := RunHook(ctx, hook, wt.Path, d.out()); err != nil {
			fmt.Fprintf(d.out(), "warning: after_issue hook failed for issue #%d: %v\n", issue.ID, err)
		}
	}
	return res
}

// MergeBack merges every successful branch into integrationBranch in dependency
// order, flips merged issues to status=done, marks merge conflicts as blocked,
// and finally cleans up the worktrees.
//
// All git operations target a dedicated per-slug integration worktree at
// {worktree_base}/{repoName}-wt/{slug}/_integration. The parent repo's HEAD is
// never modified, so concurrent dispatchers against different slugs in the
// same clone don't race on HEAD and the user can keep working in the parent
// while dispatch runs. The integration worktree is reset on every entry
// (merge --abort, hard reset, clean -fdx) so a prior run that crashed
// mid-merge doesn't poison the next one.
func (d *Dispatcher) MergeBack(ctx context.Context, slug string, results []SubResult, integrationBranch string) error {
	successful := successfulInDepOrder(ctx, results, d.plansRepo(), slug)
	if len(successful) == 0 {
		// Nothing to merge — skip the iwt setup so an all-failed batch doesn't
		// pay the worktree-create cost. GC also short-circuits because no
		// branch is in `merged`.
		return nil
	}

	iwt, err := worktree.CreateIntegration(ctx, d.Root, d.Config, slug)
	if err != nil {
		return fmt.Errorf("creating integration worktree: %w", err)
	}
	if err := worktree.ResetIntegration(ctx, iwt.Path, integrationBranch); err != nil {
		return fmt.Errorf("resetting integration worktree: %w", err)
	}
	// linkPlansDir runs AFTER ResetIntegration because `clean -fdx` would
	// otherwise delete the symlinks just created.
	if err := linkPlansDir(d.Root, iwt.Path); err != nil {
		return fmt.Errorf("linking plans dir into integration worktree: %w", err)
	}

	// Track which branches were successfully merged into integration; only those
	// are safe for GC to delete. Branches whose merge conflicted (now blocked)
	// hold the only copy of committed work and must be preserved.
	merged := make(map[string]bool, len(successful))
	for _, r := range successful {
		mergeOut, mergeErr := runGitOutput(ctx, iwt.Path, "merge", "--no-ff", "-m", fmt.Sprintf("merge issue #%d", r.IssueID), r.Branch)
		if mergeErr != nil {
			_ = runGit(ctx, iwt.Path, "merge", "--abort")
			d.markBlockedAndWarn(slug, r.IssueID, fmt.Sprintf("merge conflict on branch %s:\n%s", r.Branch, mergeOut))
			continue
		}
		merged[r.Branch] = true
		if err := d.statusOwner().Transition(ctx, slug, r.IssueID, []status.Status{status.StatusInReview}, status.StatusDone, ""); err != nil {
			if errors.Is(err, status.ErrAlreadyInState) {
				continue
			}
			return fmt.Errorf("flipping issue #%d to done: %w", r.IssueID, err)
		}
	}

	// GC runs from the iwt so `branch -d`'s reachability check resolves against
	// integration's HEAD (which now contains the merge commits), not the
	// parent's HEAD (which is some unrelated user-facing branch). includeIntegration
	// is false here — the iwt is the cwd we're operating from, and an in-flight
	// run still needs it for the next batch.
	if _, err := worktree.GC(ctx, iwt.Path, slug, merged, d.out(), false); err != nil {
		return fmt.Errorf("worktree gc: %w", err)
	}

	if hook := d.Config.Hooks.AfterBatch; hook != "" {
		if _, err := RunHook(ctx, hook, iwt.Path, d.out()); err != nil {
			fmt.Fprintf(d.out(), "warning: after_batch hook failed: %v\n", err)
		}
	}
	return nil
}

// blockFromStatuses is the set of statuses from which a dispatch failure may
// transition an issue to blocked. Backlog is included because ReadyAFK accepts
// backlog issues: a failure before the sub-agent flips backlog→todo→in-progress
// would otherwise leave the issue stuck at backlog while CAS rejects every
// block attempt — the dispatch loop would then re-pick the same issue forever.
var blockFromStatuses = []status.Status{
	status.StatusBacklog, status.StatusTodo, status.StatusInProgress, status.StatusInReview,
}

// markBlockedAndWarn flips the issue to blocked via the status owner and warns
// to stderr if the transition fails. Callers are already on a failure path; a
// warn-and-continue is preferable to bubbling the error and masking the
// original cause. ErrAlreadyInState (issue already blocked) is silently
// ignored — that's a no-op the operator doesn't need to see.
//
// The transition uses a fresh ctx detached from the parent: a canceled parent
// (Ctrl-C, or the subprocess_timeout when the merge-conflict path runs after
// a SIGKILL'd run) would otherwise drop the blocked-state write and leave the
// issue in-progress for the next loop to re-pick.
func (d *Dispatcher) markBlockedAndWarn(slug string, id int, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), blockTransitionTimeout)
	defer cancel()
	err := d.statusOwner().Transition(ctx, slug, id,
		blockFromStatuses,
		status.StatusBlocked, reason)
	if err == nil || errors.Is(err, status.ErrAlreadyInState) {
		return
	}
	fmt.Fprintf(d.out(), "warning: failed to mark issue #%d blocked (%s); issue may re-dispatch on next loop\n", id, err)
}

// cleanupWorktree removes a worktree leaked by a runOne failure between
// worktree.Create and RunSubprocess, so a failed claim/link/hook/prompt does
// not leave an orphaned worktree on disk. A fresh context is used so a
// canceled parent ctx (Ctrl-C) still tears the worktree down. A removal
// failure is warned but not returned — the caller is already surfacing the
// original failure and must not have it masked.
func (d *Dispatcher) cleanupWorktree(path string) {
	d.gitMu.Lock()
	err := worktree.Remove(context.Background(), d.Root, path)
	d.gitMu.Unlock()
	if err != nil {
		fmt.Fprintf(d.out(), "warning: failed to remove leaked worktree %q: %v\n", path, err)
	}
}

func successfulInDepOrder(ctx context.Context, results []SubResult, plans *planrepo.Plans, slug string) []SubResult {
	successByID := make(map[int]SubResult, len(results))
	for _, r := range results {
		if r.Success {
			successByID[r.IssueID] = r
		}
	}

	issues, err := snapshotPlanIssues(ctx, plans, slug)
	if err != nil {
		// fall back to result order if snapshot fails
		out := make([]SubResult, 0, len(successByID))
		for _, r := range results {
			if _, ok := successByID[r.IssueID]; ok {
				out = append(out, r)
			}
		}
		return out
	}

	depthByID := computeDepth(issues)

	successful := make([]SubResult, 0, len(successByID))
	for _, r := range results {
		if _, ok := successByID[r.IssueID]; ok {
			successful = append(successful, r)
		}
	}
	sort.SliceStable(successful, func(i, j int) bool {
		return depthByID[successful[i].IssueID] < depthByID[successful[j].IssueID]
	})
	return successful
}

func computeDepth(issues []schema.Issue) map[int]int {
	byID := make(map[int]schema.Issue, len(issues))
	for _, iss := range issues {
		byID[iss.ID] = iss
	}
	depth := make(map[int]int, len(issues))
	var visit func(id int) int
	visit = func(id int) int {
		if d, ok := depth[id]; ok {
			return d
		}
		iss, ok := byID[id]
		if !ok || len(iss.BlockedBy) == 0 {
			depth[id] = 0
			return 0
		}
		max := 0
		for _, b := range iss.BlockedBy {
			d := visit(b)
			if d+1 > max {
				max = d + 1
			}
		}
		depth[id] = max
		return max
	}
	for _, iss := range issues {
		visit(iss.ID)
	}
	return depth
}

// pendingMergeBack returns synthetic SubResults for issues stuck in `in-review`
// with a branch set. These are the recoverable remnants of a prior dispatch
// that completed a sub-agent (and the `pba complete` status flip) but exited
// before MergeBack could merge the branch and flip status to done. Feeding
// these back into MergeBack is safe: an already-merged branch is a no-op
// (`git merge --no-ff` reports "Already up to date.") and Transition still
// flips in-review → done; an unmerged branch gets merged for the first time;
// a missing/conflicting branch is routed to markBlockedAndWarn so the issue
// surfaces as blocked instead of silently re-stalling.
//
// In-review issues without a branch are skipped — those imply a human flipped
// status by hand and there's no branch to merge.
func pendingMergeBack(issues []schema.Issue) []SubResult {
	var results []SubResult
	for _, iss := range issues {
		if iss.Status != "in-review" {
			continue
		}
		if iss.Branch == nil || *iss.Branch == "" {
			continue
		}
		results = append(results, SubResult{
			IssueID: iss.ID,
			Success: true,
			Branch:  *iss.Branch,
		})
	}
	return results
}

func hitlOnlyRemaining(issues []schema.Issue) bool {
	hasHITL := false
	for _, iss := range issues {
		switch iss.Status {
		case "done", "canceled", "in-review":
			continue
		}
		if iss.HasLabel("AFK") && !iss.HasLabel("HITL") {
			return false
		}
		if iss.HasLabel("HITL") {
			hasHITL = true
		}
	}
	return hasHITL
}

// blockedSummary describes the blocked issues in a snapshot for the "stuck"
// error. It deliberately does not reuse plan.Resolve's BlockedCount: that field
// counts only dependency-blocked issues (blocked status AND unresolved deps),
// so an issue blocked operationally — a killed sub-agent, a merge conflict, a
// failed hook — has no open deps and is undercounted, producing the misleading
// "0 blocked" on a run that just blocked an issue. The stuck path needs the
// literal count, plus the IDs so the operator knows what to unblock.
func blockedSummary(issues []schema.Issue) string {
	var ids []string
	for _, iss := range issues {
		if iss.Status == "blocked" {
			ids = append(ids, fmt.Sprintf("#%d", iss.ID))
		}
	}
	if len(ids) == 0 {
		return "0 blocked"
	}
	return fmt.Sprintf("%d blocked (issues %s)", len(ids), strings.Join(ids, ", "))
}

// setupError marks a runOne failure that happened during environment setup
// (worktree create, claim, skill linking, before_issue hook, prompt build)
// rather than inside the sub-agent. When every issue in a batch fails setup,
// the dispatch loop surfaces it as one environment error instead of looping
// into the misleading "stuck: N blocked" path, where the shared root cause is
// buried in per-issue blocked notes.
type setupError struct {
	reason   string
	groupKey string
}

func (e *setupError) Error() string { return e.reason }

// newSetupError builds a setupError, folding worktreePath (when known) out of
// the grouping key. Without this the per-issue worktree path embedded in most
// setup reasons makes identical causes look distinct, defeating the collapse.
func newSetupError(reason, worktreePath string) *setupError {
	groupKey := reason
	if worktreePath != "" {
		groupKey = strings.ReplaceAll(reason, worktreePath, "<worktree>")
	}
	return &setupError{reason: reason, groupKey: groupKey}
}

// allSetupFailed reports whether every result is a setup-phase failure — i.e.
// no sub-agent ever ran. The returned string is the shared cause when all
// results fail the same way (grouped by setupError.groupKey, which folds out the
// per-issue worktree path), else a per-cause tally. Returns ("", false) for an
// empty batch or any result that is a success or a non-setup (sub-agent) failure.
func allSetupFailed(results []SubResult) (string, bool) {
	if len(results) == 0 {
		return "", false
	}
	reasons := make(map[string]int)
	for _, r := range results {
		var se *setupError
		if !errors.As(r.Err, &se) {
			return "", false
		}
		reasons[se.groupKey]++
	}
	if len(reasons) == 1 {
		for reason := range reasons {
			return reason, true
		}
	}
	parts := make([]string, 0, len(reasons))
	for reason, n := range reasons {
		parts = append(parts, fmt.Sprintf("%d× %s", n, reason))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; "), true
}

func (d *Dispatcher) printHITLSummary(issues []schema.Issue) {
	fmt.Fprintln(d.out(), "HITL: the following issues require human input:")
	for _, iss := range issues {
		switch iss.Status {
		case "done", "canceled", "in-review":
			continue
		}
		if iss.HasLabel("HITL") {
			fmt.Fprintf(d.out(), "  - #%d %s (%s)\n", iss.ID, iss.Name, iss.Status)
		}
	}
}

// ensureIntegrationBranch returns the branch name dispatch will merge into.
// "direct" → base ref, "integration" → user/<slug> forked off the base if missing.
//
// The base is d.Base when set (already validated by the CLI as a commit-ish),
// otherwise the auto-detected repo default branch. When d.Base is set and the
// integration branch already exists from a prior run, the flag is ignored and
// a warning is emitted — re-forking would clobber merged work, and silently
// ignoring the explicit flag would mislead the operator.
func (d *Dispatcher) ensureIntegrationBranch(ctx context.Context, slug string) (string, error) {
	base := d.Base
	if base == "" {
		auto, err := defaultBranch(ctx, d.Root)
		if err != nil {
			return "", err
		}
		base = auto
	}

	if d.strategy() == "direct" {
		return base, nil
	}

	user, err := gitUser(ctx, d.Root)
	if err != nil {
		return "", err
	}
	branch := fmt.Sprintf("%s/%s", user, slug)

	exists, err := branchExists(ctx, d.Root, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		if err := runGit(ctx, d.Root, "branch", branch, base); err != nil {
			return "", fmt.Errorf("creating integration branch %q: %w", branch, err)
		}
		return branch, nil
	}
	if d.Base != "" {
		fmt.Fprintf(d.out(), "warning: integration branch %q already exists; --base %q ignored\n", branch, d.Base)
	}
	return branch, nil
}

func defaultBranch(ctx context.Context, root string) (string, error) {
	if out, err := exec.CommandContext(ctx, "git", "-C", root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		ref := strings.TrimSpace(string(out))
		if strings.HasPrefix(ref, "origin/") {
			return strings.TrimPrefix(ref, "origin/"), nil
		}
	}
	for _, name := range []string{"main", "master"} {
		ok, _ := branchExists(ctx, root, name)
		if ok {
			return name, nil
		}
	}
	// Final fallback: current branch. `rev-parse --abbrev-ref HEAD` returns the
	// literal "HEAD" when detached, which would be propagated as a branch name
	// and explode at `git branch <user>/<slug> HEAD`. Use `symbolic-ref` instead;
	// it errors cleanly in detached state.
	out, err := exec.CommandContext(ctx, "git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("determining default branch (HEAD detached or no branches): %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", fmt.Errorf("determining default branch: empty HEAD ref")
	}
	return branch, nil
}

func branchExists(ctx context.Context, root, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func gitUser(ctx context.Context, root string) (string, error) {
	if email, err := gitConfig(ctx, root, "user.email"); err == nil && email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at], nil
		}
		return email, nil
	}
	name, err := gitConfig(ctx, root, "user.name")
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(name), "-"), nil
}

func gitConfig(ctx context.Context, root, key string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "config", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).CombinedOutput()
	return string(out), err
}

// linkPlansDir provisions the parent's .plan-bender/ and .claude/skills/ into
// the worktree. Both are typically gitignored, so a fresh checkout lacks them —
// the sub-agent's `pba complete` and BuildPrompt's skill lookup both depend on
// them.
func linkPlansDir(parent, worktreePath string) error {
	if err := linkDir(parent, worktreePath, ".plan-bender"); err != nil {
		return err
	}
	return linkSkills(parent, worktreePath)
}

// linkDir symlinks parent/rel into the worktree as a whole directory. A real
// (non-symlink) path already at the destination is an error: .plan-bender is
// the dispatch loop's single on-disk persistence boundary, and a real one in
// the worktree would route the sub-agent's status writes away from the parent
// and silently break the loop. A missing source is skipped.
func linkDir(parent, worktreePath, rel string) error {
	src := filepath.Join(parent, rel)
	if _, err := os.Stat(src); err != nil {
		return nil
	}
	dst := filepath.Join(worktreePath, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s exists as a real path in worktree; sub-agent writes would diverge from the parent", rel)
		}
		_ = os.Remove(dst)
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("symlinking %s -> %s: %w", dst, src, err)
	}
	return nil
}

// linkSkills provisions the parent's .claude/skills children into the worktree
// one symlink at a time, rather than symlinking the directory whole. A repo
// that git-tracks even one skill (e.g. a project-bundled skill) makes git
// recreate a real .claude/skills in every fresh worktree, which makes a
// whole-dir symlink impossible — so the gitignored bender-* skills the prompt
// builder needs would never arrive. Linking per child adds the missing skills
// while leaving committed ones in place. Children already present (committed or
// previously linked) are left untouched, so re-entry is idempotent.
//
// After provisioning, the skill BuildPrompt renders must be present, or this
// fails here with a clear setup error instead of deep in prompt-build.
func linkSkills(parent, worktreePath string) error {
	srcDir := filepath.Join(parent, ".claude", "skills")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading skills dir %s: %w", srcDir, err)
	}
	dstDir := filepath.Join(worktreePath, ".claude", "skills")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		dst := filepath.Join(dstDir, e.Name())
		if _, err := os.Lstat(dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspecting %s: %w", dst, err)
		}
		if err := os.Symlink(filepath.Join(srcDir, e.Name()), dst); err != nil {
			return fmt.Errorf("symlinking %s: %w", dst, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dstDir, requiredSkill, "SKILL.md")); err != nil {
		return fmt.Errorf("required skill %q missing from worktree after provisioning %s; is it present in the parent's .claude/skills?", requiredSkill, dstDir)
	}
	return nil
}
