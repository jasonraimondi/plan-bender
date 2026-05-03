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

// ErrHITLOnly signals that the dispatch loop ended because every remaining
// issue requires human input. The CLI maps this to exit code 2.
var ErrHITLOnly = errors.New("only HITL issues remain")

// Dispatcher orchestrates the full implementation loop for a plan: resolve →
// worktrees → spawn claude subprocesses → merge → cleanup, repeating until
// all_done or HITL-only.
type Dispatcher struct {
	Config config.Config
	Root   string // absolute path to the parent repo

	// PlansDir overrides Config.PlansDir when set; mainly for tests.
	PlansDir string

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
	// Run goes through the same lock-aware adapter without re-allocating.
	ownerOnce sync.Once
	owner     *status.Owner

	// plansOnce + plans memoize the planrepo.Plans handle used for resolver
	// and merge-order snapshots. Sharing one handle across a Run keeps every
	// read path on the same persistence boundary as status writes.
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
// the same lock without deadlocking.
func (d *Dispatcher) snapshotIssues(slug string) ([]schema.IssueYaml, error) {
	return snapshotPlanIssues(d.plansRepo(), slug)
}

func snapshotPlanIssues(plans *planrepo.Plans, slug string) ([]schema.IssueYaml, error) {
	sess, err := plans.Open(slug)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	issues := sess.Snapshot().Issues
	cp := make([]schema.IssueYaml, len(issues))
	copy(cp, issues)
	return cp, nil
}

// statusOwner returns the lazily-constructed status.Owner backed by the
// production prodStatusStore wired to d.plansDir(). All status writes during
// a Run flow through this single Owner.
func (d *Dispatcher) statusOwner() *status.Owner {
	d.ownerOnce.Do(func() {
		d.owner = status.New(newProdStatusStore(d.plansDir(), d.Config))
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
	integrationBranch, err := d.ensureIntegrationBranch(ctx, slug)
	if err != nil {
		return fmt.Errorf("setting up integration branch: %w", err)
	}

	for {
		issues, err := d.snapshotIssues(slug)
		if err != nil {
			return fmt.Errorf("loading issues: %w", err)
		}

		res := plan.Resolve(issues)
		if res.AllDone {
			return nil
		}

		batch := plan.ReadyAFK(issues)
		if len(batch) == 0 {
			if hitlOnlyRemaining(issues) {
				d.printHITLSummary(issues)
				return ErrHITLOnly
			}
			return fmt.Errorf("dispatch stuck: no AFK candidates ready and no HITL issues; %d blocked", res.BlockedCount)
		}

		results, err := d.RunBatch(ctx, slug, batch, integrationBranch)
		if err != nil {
			return fmt.Errorf("running batch: %w", err)
		}

		if err := d.MergeBack(ctx, slug, results, integrationBranch); err != nil {
			return fmt.Errorf("merging batch: %w", err)
		}
	}
}

// RunBatch fans out one goroutine per issue, each creating a worktree off
// integrationBranch, rendering a prompt, and running a claude subprocess.
// Results come back via a buffered channel and are returned in input order.
func (d *Dispatcher) RunBatch(ctx context.Context, slug string, issues []schema.IssueYaml, integrationBranch string) ([]SubResult, error) {
	logDir := filepath.Join(d.Root, ".plan-bender", "logs", slug)

	results := make([]SubResult, len(issues))
	var wg sync.WaitGroup

	for i := range issues {
		wg.Add(1)
		go func(idx int, issue schema.IssueYaml) {
			defer wg.Done()
			results[idx] = d.runOne(ctx, slug, issue, logDir, integrationBranch)
		}(i, issues[i])
	}

	wg.Wait()
	return results, nil
}

func (d *Dispatcher) runOne(ctx context.Context, slug string, issue schema.IssueYaml, logDir, integrationBranch string) SubResult {
	d.gitMu.Lock()
	wt, err := worktree.Create(ctx, d.Root, slug, issue.ID, issue.Slug, integrationBranch)
	d.gitMu.Unlock()
	if err != nil {
		reason := fmt.Sprintf("creating worktree: %v", err)
		d.markBlockedAndWarn(ctx, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Err: errors.New(reason)}
	}

	// Atomic claim: stamp branch + flip to in-progress in the YAML through the
	// canonical struct round-trip path. Without this the sub-agent's prompt
	// still shows status: backlog/todo with branch: null, and the implement-issue
	// skill instructs it to "set branch" by textual edit — Edit on a non-unique
	// substring or a naive append produces duplicate `branch:` keys, which yaml.v3
	// then rejects on every subsequent Load.
	if err := d.statusOwner().Claim(ctx, slug, issue.ID, wt.Branch, "dispatch worktree"); err != nil && !errors.Is(err, status.ErrAlreadyInState) {
		reason := fmt.Sprintf("claiming issue: %v", err)
		d.markBlockedAndWarn(ctx, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
	}
	// Mirror the on-disk update into the in-memory copy so BuildPrompt embeds
	// the post-claim state. Otherwise the sub-agent's prompt shows backlog/null
	// and the skill body talks it into re-stamping the same fields by hand.
	issue.Status = string(status.StatusInProgress)
	branchCopy := wt.Branch
	issue.Branch = &branchCopy

	if err := linkPlansDir(d.Root, wt.Path); err != nil {
		reason := fmt.Sprintf("linking plans dir: %v", err)
		d.markBlockedAndWarn(ctx, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
	}

	if hook := d.Config.Hooks.BeforeIssue; hook != "" {
		if stderr, err := RunHook(ctx, hook, wt.Path, d.out()); err != nil {
			reason := fmt.Sprintf("before_issue hook failed: %v\n%s", err, stderr)
			d.markBlockedAndWarn(ctx, slug, issue.ID, reason)
			return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
		}
	}

	prompt, err := BuildPrompt(wt.Path, issue)
	if err != nil {
		reason := fmt.Sprintf("building prompt: %v", err)
		d.markBlockedAndWarn(ctx, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
	}

	subCtx, cancel := context.WithTimeout(ctx, d.Config.Pipeline.ResolvedSubprocessTimeout())
	defer cancel()
	res := RunSubprocess(subCtx, d.statusOwner(), slug, issue, prompt, wt.Path, d.plansDir(), logDir, d.out())
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
// To avoid silently leaving the user's working tree on the integration branch,
// MergeBack captures the parent's HEAD before checkout and restores it on exit.
// It refuses to run if the parent has uncommitted changes (the checkout would
// either fail or leak changes onto the integration branch).
func (d *Dispatcher) MergeBack(ctx context.Context, slug string, results []SubResult, integrationBranch string) (err error) {
	successful := successfulInDepOrder(results, d.plansRepo(), slug)
	if len(successful) == 0 {
		// Nothing to merge — skip the dirty-check / HEAD swap so a dirty parent
		// doesn't surface an error that masks the real (all-failed) cause. GC
		// also short-circuits because no branch is in `merged`.
		return nil
	}

	dirty, dirtyErr := worktreeDirty(ctx, d.Root)
	if dirtyErr != nil {
		return fmt.Errorf("checking parent worktree state: %w", dirtyErr)
	}
	if dirty {
		return fmt.Errorf("refusing to merge: parent repo at %s has uncommitted changes; commit or stash before dispatch", d.Root)
	}

	origHEAD, err := captureHEAD(ctx, d.Root)
	if err != nil {
		return fmt.Errorf("capturing parent HEAD: %w", err)
	}
	defer func() {
		// Use a fresh context so a canceled parent ctx (Ctrl-C) still gets the
		// user's branch restored rather than leaving them on integration.
		if restoreErr := restoreHEAD(context.Background(), d.Root, origHEAD); restoreErr != nil {
			fmt.Fprintf(d.out(), "warning: failed to restore parent HEAD to %q: %v\n", origHEAD, restoreErr)
			if err == nil {
				err = fmt.Errorf("restoring parent HEAD to %q: %w", origHEAD, restoreErr)
			}
		}
	}()

	if err := runGit(ctx, d.Root, "checkout", integrationBranch); err != nil {
		return fmt.Errorf("checking out %s: %w", integrationBranch, err)
	}

	// Track which branches were successfully merged into integration; only those
	// are safe for GC to delete. Branches whose merge conflicted (now blocked)
	// hold the only copy of committed work and must be preserved.
	merged := make(map[string]bool, len(successful))
	for _, r := range successful {
		mergeOut, mergeErr := runGitOutput(ctx, d.Root, "merge", "--no-ff", "-m", fmt.Sprintf("merge issue #%d", r.IssueID), r.Branch)
		if mergeErr != nil {
			_ = runGit(ctx, d.Root, "merge", "--abort")
			d.markBlockedAndWarn(ctx, slug, r.IssueID, fmt.Sprintf("merge conflict on branch %s:\n%s", r.Branch, mergeOut))
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

	if _, err := worktree.GC(ctx, d.Root, slug, merged, d.out()); err != nil {
		return fmt.Errorf("worktree gc: %w", err)
	}

	if hook := d.Config.Hooks.AfterBatch; hook != "" {
		if _, err := RunHook(ctx, hook, d.Root, d.out()); err != nil {
			fmt.Fprintf(d.out(), "warning: after_batch hook failed: %v\n", err)
		}
	}
	return nil
}

// captureHEAD returns the current branch name (e.g. "main") if HEAD is on a
// branch, or the commit SHA if detached.
func captureHEAD(ctx context.Context, root string) (string, error) {
	if out, err := exec.CommandContext(ctx, "git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD").Output(); err == nil {
		ref := strings.TrimSpace(string(out))
		if ref != "" {
			return ref, nil
		}
	}
	out, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func restoreHEAD(ctx context.Context, root, ref string) error {
	if ref == "" {
		return nil
	}
	return runGit(ctx, root, "checkout", ref)
}

// worktreeDirty returns true if the parent repo has tracked-file modifications
// that `git checkout` would refuse to carry or carry silently. Untracked files
// (e.g. gitignored .plan-bender/ caches) are ignored — `git checkout` doesn't
// move them.
func worktreeDirty(ctx context.Context, root string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "diff-index", "--quiet", "HEAD", "--")
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// markBlockedAndWarn flips the issue to blocked via the status owner and warns
// to stderr if the transition fails. Callers are already on a failure path; a
// warn-and-continue is preferable to bubbling the error and masking the
// original cause. ErrAlreadyInState (issue already blocked) is silently
// ignored — that's a no-op the operator doesn't need to see.
//
// Backlog is included in the from-set because ReadyAFK accepts backlog issues
// and a runOne failure before the sub-agent has a chance to flip
// backlog→todo→in-progress would otherwise leave the issue stuck at backlog
// while CAS rejects every block attempt — the dispatch loop would then re-pick
// the same issue forever (the popar-py CAS-loop bug).
func (d *Dispatcher) markBlockedAndWarn(ctx context.Context, slug string, id int, reason string) {
	err := d.statusOwner().Transition(ctx, slug, id,
		[]status.Status{status.StatusBacklog, status.StatusTodo, status.StatusInProgress, status.StatusInReview},
		status.StatusBlocked, reason)
	if err == nil || errors.Is(err, status.ErrAlreadyInState) {
		return
	}
	fmt.Fprintf(d.out(), "warning: failed to mark issue #%d blocked (%s); issue may re-dispatch on next loop\n", id, err)
}

func successfulInDepOrder(results []SubResult, plans *planrepo.Plans, slug string) []SubResult {
	successByID := make(map[int]SubResult, len(results))
	for _, r := range results {
		if r.Success {
			successByID[r.IssueID] = r
		}
	}

	issues, err := snapshotPlanIssues(plans, slug)
	if err != nil {
		// fall back to result order if we can't load (tests covered)
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

func computeDepth(issues []schema.IssueYaml) map[int]int {
	byID := make(map[int]schema.IssueYaml, len(issues))
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

func hitlOnlyRemaining(issues []schema.IssueYaml) bool {
	hasHITL := false
	for _, iss := range issues {
		switch iss.Status {
		case "done", "canceled", "in-review":
			continue
		}
		if hasLabel(iss.Labels, "AFK") && !hasLabel(iss.Labels, "HITL") {
			return false
		}
		if hasLabel(iss.Labels, "HITL") {
			hasHITL = true
		}
	}
	return hasHITL
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func (d *Dispatcher) printHITLSummary(issues []schema.IssueYaml) {
	fmt.Fprintln(d.out(), "HITL: the following issues require human input:")
	for _, iss := range issues {
		switch iss.Status {
		case "done", "canceled", "in-review":
			continue
		}
		if hasLabel(iss.Labels, "HITL") {
			fmt.Fprintf(d.out(), "  - #%d %s (%s)\n", iss.ID, iss.Name, iss.Status)
		}
	}
}

// ensureIntegrationBranch returns the branch name dispatch will merge into.
// "direct" → repo default branch, "integration" → user/<slug> created off default if missing.
func (d *Dispatcher) ensureIntegrationBranch(ctx context.Context, slug string) (string, error) {
	defaultBranch, err := defaultBranch(ctx, d.Root)
	if err != nil {
		return "", err
	}

	if d.strategy() == "direct" {
		return defaultBranch, nil
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
		if err := runGit(ctx, d.Root, "branch", branch, defaultBranch); err != nil {
			return "", fmt.Errorf("creating integration branch %q: %w", branch, err)
		}
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

// linkPlansDir symlinks the parent's .plan-bender/ and .claude/skills/ dirs
// into the worktree. Both are typically gitignored, so a fresh worktree
// checkout doesn't have them — sub-agent calls to `pba complete` and
// BuildPrompt's skill lookup both depend on these.
func linkPlansDir(parent, worktreePath string) error {
	for _, rel := range []string{".plan-bender", filepath.Join(".claude", "skills")} {
		src := filepath.Join(parent, rel)
		dst := filepath.Join(worktreePath, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if info, err := os.Lstat(dst); err == nil {
			// Only nuke a pre-existing symlink. A real directory at dst is the
			// user's data — refuse to clobber it; let Symlink fail with EEXIST.
			if info.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(dst)
			}
		}
		if err := os.Symlink(src, dst); err != nil {
			return fmt.Errorf("symlinking %s -> %s: %w", dst, src, err)
		}
	}
	return nil
}
