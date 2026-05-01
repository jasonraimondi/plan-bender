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

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/schema"
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
	if d.Out == nil {
		return os.Stdout
	}
	return d.Out
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
	integrationBranch, err := d.ensureIntegrationBranch(slug)
	if err != nil {
		return fmt.Errorf("setting up integration branch: %w", err)
	}

	for {
		issues, err := plan.LoadIssues(d.plansDir(), slug)
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

		results, err := d.RunBatch(ctx, slug, batch)
		if err != nil {
			return fmt.Errorf("running batch: %w", err)
		}

		if err := d.MergeBack(slug, results, integrationBranch); err != nil {
			return fmt.Errorf("merging batch: %w", err)
		}
	}
}

// RunBatch fans out one goroutine per issue, each creating a worktree,
// rendering a prompt, and running a claude subprocess. Results come back via
// a buffered channel and are returned in input order.
func (d *Dispatcher) RunBatch(ctx context.Context, slug string, issues []schema.IssueYaml) ([]SubResult, error) {
	logDir := filepath.Join(d.Root, ".plan-bender", "logs", slug)

	results := make([]SubResult, len(issues))
	var wg sync.WaitGroup

	for i := range issues {
		wg.Add(1)
		go func(idx int, issue schema.IssueYaml) {
			defer wg.Done()
			results[idx] = d.runOne(ctx, slug, issue, logDir)
		}(i, issues[i])
	}

	wg.Wait()
	return results, nil
}

func (d *Dispatcher) runOne(ctx context.Context, slug string, issue schema.IssueYaml, logDir string) SubResult {
	store := backend.NewProdPlanStore(d.plansDir())

	d.gitMu.Lock()
	wt, err := worktree.Create(d.Root, slug, issue.ID, issue.Slug)
	d.gitMu.Unlock()
	if err != nil {
		reason := fmt.Sprintf("creating worktree: %v", err)
		d.markIssueBlocked(store, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Err: errors.New(reason)}
	}

	if err := linkPlansDir(d.Root, wt.Path); err != nil {
		reason := fmt.Sprintf("linking plans dir: %v", err)
		d.markIssueBlocked(store, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
	}

	if hook := d.Config.Hooks.BeforeIssue; hook != "" {
		if stderr, err := RunHook(hook, wt.Path, d.out()); err != nil {
			reason := fmt.Sprintf("before_issue hook failed: %v\n%s", err, stderr)
			d.markIssueBlocked(store, slug, issue.ID, reason)
			return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
		}
	}

	prompt, err := BuildPrompt(wt.Path, issue)
	if err != nil {
		reason := fmt.Sprintf("building prompt: %v", err)
		d.markIssueBlocked(store, slug, issue.ID, reason)
		return SubResult{IssueID: issue.ID, Branch: wt.Branch, Err: errors.New(reason)}
	}

	res := RunSubprocess(ctx, slug, issue, prompt, wt.Path, d.plansDir(), logDir, d.out())
	res.Branch = wt.Branch

	if hook := d.Config.Hooks.AfterIssue; hook != "" {
		if _, err := RunHook(hook, wt.Path, d.out()); err != nil {
			fmt.Fprintf(d.out(), "warning: after_issue hook failed for issue #%d: %v\n", issue.ID, err)
		}
	}
	return res
}

// MergeBack merges every successful branch into integrationBranch in dependency
// order, flips merged issues to status=done, marks merge conflicts as blocked,
// and finally cleans up the worktrees.
func (d *Dispatcher) MergeBack(slug string, results []SubResult, integrationBranch string) error {
	if err := runGit(d.Root, "checkout", integrationBranch); err != nil {
		return fmt.Errorf("checking out %s: %w", integrationBranch, err)
	}

	successful := successfulInDepOrder(results, d.plansDir(), slug)
	store := backend.NewProdPlanStore(d.plansDir())

	for _, r := range successful {
		mergeOut, err := runGitOutput(d.Root, "merge", "--no-ff", "-m", fmt.Sprintf("merge issue #%d", r.IssueID), r.Branch)
		if err != nil {
			_ = runGit(d.Root, "merge", "--abort")
			d.markIssueBlocked(store, slug, r.IssueID, fmt.Sprintf("merge conflict on branch %s:\n%s", r.Branch, mergeOut))
			continue
		}
		if err := d.markIssueDone(store, slug, r.IssueID); err != nil {
			fmt.Fprintf(d.out(), "warning: failed to flip issue #%d to done: %v\n", r.IssueID, err)
		}
	}

	if _, err := worktree.GC(d.Root, slug); err != nil {
		return fmt.Errorf("worktree gc: %w", err)
	}

	if hook := d.Config.Hooks.AfterBatch; hook != "" {
		if _, err := RunHook(hook, d.Root, d.out()); err != nil {
			fmt.Fprintf(d.out(), "warning: after_batch hook failed: %v\n", err)
		}
	}
	return nil
}

func (d *Dispatcher) markIssueDone(store *backend.PlanStore, slug string, id int) error {
	issue, err := loadIssue(d.plansDir(), slug, id)
	if err != nil {
		return err
	}
	issue.Status = "done"
	issue.Updated = time.Now().Format("2006-01-02")
	return store.WriteIssue(slug, issue)
}

func (d *Dispatcher) markIssueBlocked(store *backend.PlanStore, slug string, id int, reason string) {
	issue, err := loadIssue(d.plansDir(), slug, id)
	if err != nil {
		return
	}
	issue.Status = "blocked"
	issue.Updated = time.Now().Format("2006-01-02")
	if issue.Notes == nil {
		issue.Notes = &reason
	} else {
		merged := *issue.Notes + "\n\n" + reason
		issue.Notes = &merged
	}
	_ = store.WriteIssue(slug, issue)
}

func successfulInDepOrder(results []SubResult, plansDir, slug string) []SubResult {
	successByID := make(map[int]SubResult, len(results))
	for _, r := range results {
		if r.Success {
			successByID[r.IssueID] = r
		}
	}

	issues, err := plan.LoadIssues(plansDir, slug)
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
func (d *Dispatcher) ensureIntegrationBranch(slug string) (string, error) {
	defaultBranch, err := defaultBranch(d.Root)
	if err != nil {
		return "", err
	}

	if d.strategy() == "direct" {
		return defaultBranch, nil
	}

	user, err := gitUser(d.Root)
	if err != nil {
		return "", err
	}
	branch := fmt.Sprintf("%s/%s", user, slug)

	exists, err := branchExists(d.Root, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		if err := runGit(d.Root, "branch", branch, defaultBranch); err != nil {
			return "", fmt.Errorf("creating integration branch %q: %w", branch, err)
		}
	}
	return branch, nil
}

func defaultBranch(root string) (string, error) {
	if out, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD").Output(); err == nil {
		ref := strings.TrimSpace(string(out))
		if strings.HasPrefix(ref, "origin/") {
			return strings.TrimPrefix(ref, "origin/"), nil
		}
	}
	for _, name := range []string{"main", "master"} {
		ok, _ := branchExists(root, name)
		if ok {
			return name, nil
		}
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("determining default branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func branchExists(root, name string) (bool, error) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func gitUser(root string) (string, error) {
	if email, err := gitConfig(root, "user.email"); err == nil && email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at], nil
		}
		return email, nil
	}
	name, err := gitConfig(root, "user.name")
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(name), "-"), nil
}

func gitConfig(root, key string) (string, error) {
	out, err := exec.Command("git", "-C", root, "config", key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runGit(dir string, args ...string) error {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runGitOutput(dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
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
		if _, err := os.Lstat(dst); err == nil {
			_ = os.RemoveAll(dst)
		}
		if err := os.Symlink(src, dst); err != nil {
			return err
		}
	}
	return nil
}
