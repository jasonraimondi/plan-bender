package planrepo

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FindIssueProject returns the slug of the plan that owns issue id. Plans are
// scanned in deterministic sorted-by-slug order; the first plan whose issues
// directory contains a file prefixed with "{id}-" wins. Returns an error when
// no plan owns the id.
//
// This is a read-only filesystem walk through the repository's FS adapter; it
// does not take the plan lock.
func (p *Plans) FindIssueProject(id int) (string, error) {
	entries, err := fs.ReadDir(p.adapters.FS, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("cannot find project for issue #%d: plans dir missing", id)
		}
		return "", fmt.Errorf("scanning plans: %w", err)
	}

	slugs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		slugs = append(slugs, e.Name())
	}
	sort.Strings(slugs)

	prefix := fmt.Sprintf("%d-", id)
	for _, slug := range slugs {
		issuesDir := filepath.Join(slug, "issues")
		issueEntries, err := fs.ReadDir(p.adapters.FS, issuesDir)
		if err != nil {
			continue
		}
		for _, e := range issueEntries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".json") {
				return slug, nil
			}
		}
	}
	return "", fmt.Errorf("cannot find project for issue #%d", id)
}

// normalizeSlug strips a leading plansDir prefix from slug. Shell tab
// completion often produces slugs that include the plansDir component (e.g.
// "jason/plans/foo" when plansDir is "./jason/plans/"); without stripping,
// the loader would join plansDir twice and fail to find the plan.
func normalizeSlug(plansDir, slug string) string {
	cleanedDir := filepath.Clean(plansDir)
	cleanedSlug := filepath.Clean(slug)
	if cleanedDir == "." || cleanedDir == "" {
		return cleanedSlug
	}
	prefix := cleanedDir + string(filepath.Separator)
	if stripped, ok := strings.CutPrefix(cleanedSlug, prefix); ok {
		return stripped
	}
	return cleanedSlug
}

// planDirExists reports whether slug has any subtree at all under plansDir.
func planDirExists(fsys fs.FS, slug string) bool {
	info, err := fs.Stat(fsys, slug)
	if err != nil {
		return false
	}
	return info.IsDir()
}
