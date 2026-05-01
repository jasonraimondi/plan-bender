package worktree

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
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
func Create(root, slug string, issueID int, issueSlug, baseRef string) (WorktreeResult, error) {
	user, err := gitUser(root)
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
	parent := filepath.Dir(root)
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	path := filepath.Join(parent, repoName+"-wt", fmt.Sprintf("%d-%s", issueID, issueSlug))

	if err := runGit(root, "branch", branch, baseRef); err != nil {
		return WorktreeResult{}, fmt.Errorf("creating branch %q off %q: %w", branch, baseRef, err)
	}
	if err := runGit(root, "worktree", "add", path, branch); err != nil {
		_ = runGit(root, "branch", "-D", branch)
		return WorktreeResult{}, fmt.Errorf("creating worktree at %q: %w", path, err)
	}
	return WorktreeResult{Path: path, Branch: branch}, nil
}

// GC removes plan-bender worktrees whose branch matches {user}/{slug}--.
//
// `safe` filters which branches GC may delete: pass an explicit set to allow
// only those branches (e.g. branches confirmed merged into integration); pass
// nil to consider every matching branch a candidate. Either way, GC uses the
// non-forcing forms of `worktree remove` and `branch -d`, so a worktree with
// uncommitted changes or a branch whose commits aren't reachable from current
// HEAD is preserved with a warning. Caller is expected to invoke GC while HEAD
// is on the integration branch so `branch -d`'s reachability check matches.
//
// Returns the list of paths actually removed. `out` receives one line per
// preserved entry; pass io.Discard to silence.
func GC(root, slug string, safe map[string]bool, out io.Writer) ([]string, error) {
	if out == nil {
		out = io.Discard
	}
	user, err := gitUser(root)
	if err != nil {
		return nil, err
	}
	prefix := fmt.Sprintf("%s/%s--", user, slug)

	entries, err := listWorktrees(root)
	if err != nil {
		return nil, err
	}

	var removed []string
	for _, e := range entries {
		if !strings.HasPrefix(e.branch, prefix) {
			continue
		}
		if safe != nil && !safe[e.branch] {
			fmt.Fprintf(out, "preserving worktree %q (branch %q not in safe set; recover manually)\n", e.path, e.branch)
			continue
		}
		if err := runGit(root, "worktree", "remove", e.path); err != nil {
			fmt.Fprintf(out, "preserving worktree %q (uncommitted changes or removal failed): %v\n", e.path, err)
			continue
		}
		if err := runGit(root, "branch", "-d", e.branch); err != nil {
			fmt.Fprintf(out, "preserving branch %q (not merged from HEAD): %v\n", e.branch, err)
			continue
		}
		removed = append(removed, e.path)
	}
	return removed, nil
}

type worktreeEntry struct {
	path   string
	branch string
}

func listWorktrees(root string) ([]worktreeEntry, error) {
	cmd := exec.Command("git", "-C", root, "worktree", "list", "--porcelain")
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
func gitUser(root string) (string, error) {
	if email, err := gitConfig(root, "user.email"); err == nil && email != "" {
		if at := strings.IndexByte(email, '@'); at > 0 {
			return email[:at], nil
		}
		return email, nil
	}
	name, err := gitConfig(root, "user.name")
	if err != nil {
		return "", fmt.Errorf("git user not configured: %w", err)
	}
	if name == "" {
		return "", fmt.Errorf("git user.name is empty")
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
