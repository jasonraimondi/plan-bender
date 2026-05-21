package worktree

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

// WorktreeResult is the output of Create.
type WorktreeResult struct {
	Path   string
	Branch string
}

// Create makes a new branch off baseRef and a git worktree at the canonical
// {repo}-wt/{id}-{issueSlug} path next to the repo root. Pass baseRef="" to
// branch off HEAD (used by the ad-hoc `pba worktree create` CLI); dispatchers
// must pass the integration branch so issue commits root from a stable base
// rather than wherever the user happened to run from.
//
// Create is idempotent against the canonical path: if the branch already
// exists with a worktree at the canonical path, the existing pair is
// returned unchanged. If the branch exists but no worktree is attached, a
// fresh worktree is added against the existing branch. The branch's commit
// history is left alone — baseRef is only consulted when the branch must
// be created. This lets dispatch's outer loop re-enter runOne after a
// crash without tripping "branch already exists" errors.
//
// ctx cancels in-flight git plumbing so Ctrl-C during dispatch tears down
// pending child processes instead of leaking them.
func Create(ctx context.Context, cfg config.Config, root, slug string, issueID int, issueSlug, baseRef string) (WorktreeResult, error) {
	user, err := gitUser(ctx, root)
	if err != nil {
		return WorktreeResult{}, err
	}

	if baseRef == "" {
		baseRef = "HEAD"
	}

	// Use "--" between the project slug and the issue id-slug to avoid a git
	// ref-hierarchy clash with the integration branch named {user}/{slug}.
	branch := fmt.Sprintf("%s/%s--%d-%s", user, slug, issueID, issueSlug)
	repoName := filepath.Base(root)
	base, err := resolveBase(cfg, root)
	if err != nil {
		return WorktreeResult{}, fmt.Errorf("resolving worktree base: %w", err)
	}
	path := filepath.Join(base, repoName+"-wt", slug, fmt.Sprintf("%d-%s", issueID, issueSlug))

	branchExists, err := branchExists(ctx, root, branch)
	if err != nil {
		return WorktreeResult{}, fmt.Errorf("checking branch %q: %w", branch, err)
	}

	existingPath, err := worktreePathForBranch(ctx, root, branch)
	if err != nil {
		return WorktreeResult{}, fmt.Errorf("inspecting worktrees: %w", err)
	}
	if existingPath != "" {
		if existingPath != path {
			return WorktreeResult{}, fmt.Errorf("branch %q already checked out at %q (expected %q); resolve manually", branch, existingPath, path)
		}
		return WorktreeResult{Path: existingPath, Branch: branch}, nil
	}

	if !branchExists {
		if err := runGit(ctx, root, "branch", branch, baseRef); err != nil {
			return WorktreeResult{}, fmt.Errorf("creating branch %q off %q: %w", branch, baseRef, err)
		}
	}
	if err := runGit(ctx, root, "worktree", "add", path, branch); err != nil {
		// Only delete the branch if we created it this call. Pre-existing branches
		// may carry committed work from a prior run that the user expects to recover.
		if !branchExists {
			_ = runGit(ctx, root, "branch", "-D", branch)
		}
		return WorktreeResult{}, fmt.Errorf("creating worktree at %q: %w", path, err)
	}
	return WorktreeResult{Path: path, Branch: branch}, nil
}

// CreateIntegration lazy-creates a per-slug integration worktree at
// {worktree_base}/{repoName}-wt/{slug}/_integration on branch {user}/{slug}.
// The integration branch must already exist (caller responsibility — typically
// dispatch's ensureIntegrationBranch). CreateIntegration only attaches a worktree
// to it. Idempotent: if a worktree at the expected path already holds the branch,
// the existing pair is returned. If the branch is checked out elsewhere, an
// error surfaces so a misconfigured layout doesn't silently produce two iwts.
func CreateIntegration(ctx context.Context, root string, cfg config.Config, slug string) (WorktreeResult, error) {
	user, err := gitUser(ctx, root)
	if err != nil {
		return WorktreeResult{}, err
	}
	branch := fmt.Sprintf("%s/%s", user, slug)
	repoName := filepath.Base(root)
	base, err := resolveBase(cfg, root)
	if err != nil {
		return WorktreeResult{}, fmt.Errorf("resolving worktree base: %w", err)
	}
	path := filepath.Join(base, repoName+"-wt", slug, "_integration")

	existingPath, err := worktreePathForBranch(ctx, root, branch)
	if err != nil {
		return WorktreeResult{}, fmt.Errorf("inspecting worktrees: %w", err)
	}
	if existingPath != "" {
		// A worktree whose directory vanished out-of-band (manual rm, evicted
		// tmpdir) keeps appearing in `git worktree list`, branch line and all,
		// flagged prunable. Returning that path would hand ResetIntegration a
		// `git -C <missing>` that aborts the whole dispatch, so stat it: if the
		// directory is gone, prune the stale entry and fall through to recreate.
		if _, statErr := os.Stat(existingPath); statErr == nil {
			if existingPath != path {
				return WorktreeResult{}, fmt.Errorf("integration branch %q already checked out at %q (expected %q); resolve manually", branch, existingPath, path)
			}
			return WorktreeResult{Path: existingPath, Branch: branch}, nil
		} else if !os.IsNotExist(statErr) {
			return WorktreeResult{}, fmt.Errorf("stat integration worktree %q: %w", existingPath, statErr)
		}
		if err := runGit(ctx, root, "worktree", "prune"); err != nil {
			return WorktreeResult{}, fmt.Errorf("pruning stale integration worktree: %w", err)
		}
	}

	if err := runGit(ctx, root, "worktree", "add", path, branch); err != nil {
		return WorktreeResult{}, fmt.Errorf("creating integration worktree at %q: %w", path, err)
	}
	return WorktreeResult{Path: path, Branch: branch}, nil
}

// ResetIntegration brings the integration worktree to a known-clean state on
// branch's tip: aborts any in-flight merge (silently ignored when no merge is
// in progress), hard-resets to branch, removes untracked files (including
// gitignored ones via -x).
//
// Called on every MergeBack entry. The integration worktree is owned by
// dispatch — no user state ever lives there — so the broad clean is safe.
// Recovers from a prior run that crashed mid-merge (stale MERGE_HEAD + dirty
// index) without operator intervention.
func ResetIntegration(ctx context.Context, path, branch string) error {
	_ = runGit(ctx, path, "merge", "--abort")
	if err := runGit(ctx, path, "reset", "--hard", branch); err != nil {
		return fmt.Errorf("resetting integration worktree at %q: %w", path, err)
	}
	if err := runGit(ctx, path, "clean", "-fdx"); err != nil {
		return fmt.Errorf("cleaning integration worktree at %q: %w", path, err)
	}
	return nil
}

// resolveBase returns the directory under which {repoName}-wt/{slug}/{leaf}
// is anchored. Inputs:
//
//   - "" → repo's parent directory (legacy layout). EvalSymlinks'd so
//     downstream string-equality checks against `git worktree list` match.
//   - absolute path → returned as-is.
//   - "~/..." → expanded against the user's home directory.
//   - "./..." or any other relative path → joined with repoRoot.
func resolveBase(cfg config.Config, repoRoot string) (string, error) {
	b := strings.TrimSpace(cfg.WorktreeBase)
	if b == "" {
		parent := filepath.Dir(repoRoot)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			parent = resolved
		}
		return parent, nil
	}
	if strings.HasPrefix(b, "~/") || b == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~ in worktree_base: %w", err)
		}
		if b == "~" {
			return home, nil
		}
		return filepath.Join(home, b[2:]), nil
	}
	if filepath.IsAbs(b) {
		return b, nil
	}
	return filepath.Join(repoRoot, b), nil
}

// branchExists reports whether refs/heads/<name> resolves in root.
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

// worktreePathForBranch returns the worktree path currently checking out
// branch, or "" if no worktree has it checked out.
func worktreePathForBranch(ctx context.Context, root, branch string) (string, error) {
	entries, err := listWorktrees(ctx, root)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.branch == branch {
			return e.path, nil
		}
	}
	return "", nil
}

// GC removes plan-bender worktrees whose branch matches {user}/{slug}--.
// When includeIntegration is true, the per-slug integration worktree on
// branch {user}/{slug} (no `--` suffix) is also a candidate; dispatch passes
// true only on the AllDone exit path so an in-flight or HITL-only run
// preserves the iwt for resumption. The CLI `pba worktree gc` always passes
// true — operators invoke it to clean up everything.
//
// `safe` filters which branches GC may delete: pass an explicit set to allow
// only those branches (e.g. branches confirmed merged into integration); pass
// nil to consider every matching branch a candidate. Either way, GC uses the
// non-forcing form of `branch -d`, so a branch whose commits aren't reachable
// from current HEAD is preserved with a warning. Caller is expected to invoke
// GC while HEAD is on the integration branch so `branch -d`'s reachability
// check matches.
//
// The integration worktree is removed when includeIntegration is true, but its
// branch is the plan's deliverable: at AllDone it hasn't been merged to the
// default branch, so `branch -d` correctly refuses and the branch is preserved
// (an informational line, not a warning). It is deleted only once the operator
// has merged it and re-runs GC from a HEAD that reaches it.
//
// `worktree remove` is non-forcing for issue worktrees (uncommitted changes
// are preserved) but forcing for the integration worktree, which is
// dispatch-owned: linkPlansDir leaves untracked symlinks the non-forcing form
// would refuse to remove, and no user state ever lives there to lose.
//
// Returns the list of paths actually removed. `out` receives one line per
// preserved entry; pass io.Discard to silence.
func GC(ctx context.Context, root, slug string, safe map[string]bool, out io.Writer, includeIntegration bool) ([]string, error) {
	if out == nil {
		out = io.Discard
	}
	user, err := gitUser(ctx, root)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s/%s--", user, slug)
	integrationBranch := fmt.Sprintf("%s/%s", user, slug)

	entries, err := listWorktrees(ctx, root)
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, e := range entries {
		isIntegration := includeIntegration && e.branch == integrationBranch
		isIssue := strings.HasPrefix(e.branch, prefix)
		if !isIssue && !isIntegration {
			continue
		}
		// safe-set filtering applies only to issue branches: the set represents
		// "branches confirmed merged into integration", a concept that has no
		// meaning for the integration branch itself.
		if isIssue && safe != nil && !safe[e.branch] {
			fmt.Fprintf(out, "preserving worktree %q (branch %q not in safe set; recover manually)\n", e.path, e.branch)
			continue
		}
		removeArgs := []string{"worktree", "remove", e.path}
		if isIntegration {
			removeArgs = []string{"worktree", "remove", "--force", e.path}
		}
		if err := runGit(ctx, root, removeArgs...); err != nil {
			fmt.Fprintf(out, "preserving worktree %q (uncommitted changes or removal failed): %v\n", e.path, err)
			continue
		}
		if err := runGit(ctx, root, "branch", "-d", e.branch); err != nil {
			// The integration branch holds the plan's merged work and is never
			// reachable from the parent's HEAD at AllDone (it isn't merged to the
			// default branch yet), so `branch -d` refusing is the expected,
			// correct outcome — not a warning-worthy anomaly. The non-forcing
			// `branch -d` still deletes it once the operator has merged it and
			// re-runs `pba worktree gc`.
			if isIntegration {
				fmt.Fprintf(out, "integration branch %q preserved (holds the plan's merged work; review and merge it, then `pba worktree gc %s`)\n", e.branch, slug)
			} else {
				fmt.Fprintf(out, "preserving branch %q (not merged from HEAD): %v\n", e.branch, err)
			}
			continue
		}
		removed = append(removed, e.path)
	}
	return removed, nil
}

// Remove deletes the git worktree at path. It cleans up a worktree that
// Create produced when a later dispatch step (Claim, linkPlansDir, a hook,
// BuildPrompt) failed before the sub-agent started — at that point the
// worktree holds no committed work worth preserving. --force is required
// because dispatch symlinks .plan-bender/ and .claude/skills/ into the
// worktree, leaving untracked entries the non-forcing form would refuse to
// remove. The branch is left intact so a re-entered runOne can reattach a
// fresh worktree to it (see Create's idempotency contract).
func Remove(ctx context.Context, root, path string) error {
	return runGit(ctx, root, "worktree", "remove", "--force", path)
}

type worktreeEntry struct {
	path   string
	branch string
}

func listWorktrees(ctx context.Context, root string) ([]worktreeEntry, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	var entries []worktreeEntry
	var cur worktreeEntry
	flush := func() {
		if cur.path != "" {
			entries = append(entries, cur)
		}
		cur = worktreeEntry{}
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			cur.path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			cur.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	flush()
	return entries, nil
}

// gitUser returns a sane branch-safe username. Prefers user.email's local part
// (typical handle) and falls back to user.name with whitespace squashed.
func gitUser(ctx context.Context, root string) (string, error) {
	if email, err := gitConfig(ctx, root, "user.email"); err == nil && email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at], nil
		}
		return email, nil
	}
	name, err := gitConfig(ctx, root, "user.name")
	if err != nil {
		return "", fmt.Errorf("git user not configured: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("git user.name is empty")
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
