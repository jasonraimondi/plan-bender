package worktree

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeResult is the output of Create.
type WorktreeResult struct {
	Path   string
	Branch string
}

// Create makes a new branch off HEAD and a git worktree at the canonical
// {repo}-wt/{id}-{issueSlug} path next to the repo root.
func Create(root, slug string, issueID int, issueSlug string) (WorktreeResult, error) {
	user, err := gitUser(root)
	if err != nil {
		return WorktreeResult{}, err
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

	if err := runGit(root, "branch", branch, "HEAD"); err != nil {
		return WorktreeResult{}, fmt.Errorf("creating branch %q: %w", branch, err)
	}
	if err := runGit(root, "worktree", "add", path, branch); err != nil {
		_ = runGit(root, "branch", "-D", branch)
		return WorktreeResult{}, fmt.Errorf("creating worktree at %q: %w", path, err)
	}
	return WorktreeResult{Path: path, Branch: branch}, nil
}

// GC removes every plan-bender worktree whose branch matches {user}/{slug}/.
// Returns the list of removed worktree paths.
func GC(root, slug string) ([]string, error) {
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
		if err := runGit(root, "worktree", "remove", "--force", e.path); err != nil {
			return removed, fmt.Errorf("removing worktree %q: %w", e.path, err)
		}
		_ = runGit(root, "branch", "-D", e.branch)
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
