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
	"time"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
	"github.com/jasonraimondi/plan-bender/internal/worktree"
)

// subprocessWaitDelay bounds how long cmd.Wait blocks after the process exits
// or after a ctx-cancel kill: once it elapses, os/exec force-kills the process
// and closes the pipe fds it owns, unblocking the I/O-copy goroutine even if a
// surviving grandchild still holds the pipe's write end. Used by RunHook.
const subprocessWaitDelay = 10 * time.Second

// blockTransitionTimeout bounds the failure-path blocked-status write. The
// transition uses a fresh ctx (detached from any caller-supplied deadline) so
// a Ctrl-C canceling a merge cannot drop the write and leave the issue
// in-progress for the next loop to re-pick.
const blockTransitionTimeout = 30 * time.Second

// SubResult is the outcome of a single sub-agent run, carried through MergeBack.
type SubResult struct {
	IssueID int
	Success bool
	Branch  string
	Err     error
}

// Dispatcher owns the merge-back internals for a plan: it merges completed
// issue branches into the integration branch in dependency order and runs the
// after_batch hook. The cold-subprocess implementation loop has been removed;
// `pba merge` is the surviving entry point.
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

	// Out is where prefixed hook stdout and warnings are streamed. Defaults to
	// os.Stdout.
	Out io.Writer

	// outOnce + outWriter memoize the synchronized writer wrapping d.Out so
	// concurrent writers share one mutex.
	outOnce   sync.Once
	outWriter io.Writer

	// ownerOnce + owner memoize the status.Owner so every status write goes
	// through one lock-aware adapter without re-allocating. The Owner wraps its
	// own planrepo.Plans handle (NewProdStatusOwner), distinct from `plans`
	// below but rooted at the same plansDir.
	ownerOnce sync.Once
	owner     *status.Owner

	// plansOnce + plans memoize the planrepo.Plans handle for the merge-order
	// snapshots and the loadIssue read in MergeBack. It does not back status
	// writes — those go through the Owner's own handle (see ownerOnce) — but all
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

func loadIssue(ctx context.Context, plans *planrepo.Plans, slug string, id int) (*schema.Issue, error) {
	sess, err := plans.OpenContext(ctx, slug)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	for i := range sess.Snapshot().Issues {
		if sess.Snapshot().Issues[i].ID == id {
			iss := sess.Snapshot().Issues[i]
			return &iss, nil
		}
	}
	return nil, fmt.Errorf("issue #%d not found in %q", id, slug)
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
		// Subject describes the merged work, not the plan-local issue number:
		// "#N" auto-links to an unrelated GitHub issue, and a bare number is
		// meaningless on the remote. Fall back to the branch name if the issue
		// can't be loaded.
		mergeMsg := r.Branch
		if iss, err := loadIssue(ctx, d.plansRepo(), slug, r.IssueID); err == nil && iss.Name != "" {
			mergeMsg = iss.Name
		}
		mergeOut, mergeErr := runGitOutput(ctx, iwt.Path, "merge", "--no-ff", "-m", mergeMsg, r.Branch)
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

// requiredSkill is the implement-issue skill every issue worktree must carry.
// linkSkills provisions it and verifies it lands so a missing skill fails at
// link time.
const requiredSkill = "bender-implement-issue"

// linkPlansDir provisions the parent's .plan-bender/ and .claude/skills/ into
// the worktree. Both are typically gitignored, so a fresh checkout lacks them —
// the sub-agent's `pba complete` and skill lookup both depend on them.
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
// whole-dir symlink impossible — so the gitignored bender-* skills the
// sub-agent needs would never arrive. Linking per child adds the missing skills
// while leaving committed ones in place. Children already present (committed or
// previously linked) are left untouched, so re-entry is idempotent.
//
// After provisioning, the required skill must be present, or this fails here
// with a clear setup error.
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
