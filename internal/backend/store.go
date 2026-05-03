package backend

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// WriteFunc is a function that writes data to a path.
type WriteFunc func(path string, data []byte, perm fs.FileMode) error

// MkdirFunc is a function that creates directories.
type MkdirFunc func(path string, perm fs.FileMode) error

// PlanStore handles reading/writing plan files with injectable I/O.
type PlanStore struct {
	root  string
	fsys  fs.FS
	write WriteFunc
	mkdir MkdirFunc
}

// NewPlanStore creates a PlanStore.
func NewPlanStore(root string, fsys fs.FS, write WriteFunc, mkdir MkdirFunc) *PlanStore {
	return &PlanStore{root: root, fsys: fsys, write: write, mkdir: mkdir}
}

// NewProdPlanStore creates a PlanStore wired with production I/O: an os
// filesystem reader, atomic temp+rename writes serialized by a flock on the
// plans dir (so concurrent sub-agents reaching plansDir via a symlink can't
// lose updates), and recursive mkdir.
func NewProdPlanStore(plansDir string) *PlanStore {
	return NewPlanStore(plansDir, prodFS(plansDir), lockedAtomicWrite(plansDir), prodMkdir)
}

// NewUnlockedPlanStore returns a prod-style PlanStore that does NOT take the
// plans-dir flock on each write. Use this only inside a held LockPlanDir
// region — composing a load-modify-write under one outer lock — to avoid
// deadlocking against the per-write lock in NewProdPlanStore.
func NewUnlockedPlanStore(plansDir string) *PlanStore {
	return NewPlanStore(plansDir, prodFS(plansDir), AtomicWrite, prodMkdir)
}

// ReadPrd reads and parses a PRD YAML file.
func (s *PlanStore) ReadPrd(slug string) (*schema.PrdYaml, error) {
	path := filepath.Join(slug, "prd.yaml")
	data, err := fs.ReadFile(s.fsys, path)
	if err != nil {
		return nil, fmt.Errorf("reading prd %s: %w", path, err)
	}
	var prd schema.PrdYaml
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("parsing prd %s: %w", path, err)
	}
	return &prd, nil
}

// ReadIssues reads all issue YAML files for a project slug.
func (s *PlanStore) ReadIssues(slug string) ([]schema.IssueYaml, error) {
	issuesDir := filepath.Join(slug, "issues")
	entries, err := fs.ReadDir(s.fsys, issuesDir)
	if err != nil {
		return nil, fmt.Errorf("listing issues in %s: %w", issuesDir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	issues := make([]schema.IssueYaml, 0, len(names))
	for _, name := range names {
		path := filepath.Join(issuesDir, name)
		data, err := fs.ReadFile(s.fsys, path)
		if err != nil {
			return nil, fmt.Errorf("reading issue %s: %w", path, err)
		}
		var issue schema.IssueYaml
		if err := yaml.Unmarshal(data, &issue); err != nil {
			return nil, fmt.Errorf("parsing issue %s: %w", path, err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// WritePrd writes a PRD YAML file.
func (s *PlanStore) WritePrd(slug string, prd *schema.PrdYaml) error {
	dir := filepath.Join(s.root, slug)
	if err := s.mkdir(dir, 0o755); err != nil {
		return err
	}
	issuesDir := filepath.Join(dir, "issues")
	if err := s.mkdir(issuesDir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(prd)
	if err != nil {
		return fmt.Errorf("marshaling prd: %w", err)
	}
	return s.write(filepath.Join(dir, "prd.yaml"), data, 0o644)
}

// WriteIssue writes an issue YAML file. The marshaled bytes are re-parsed
// before write so a regression that produces non-roundtripping YAML (e.g.
// duplicate keys from a future custom MarshalYAML) fails loudly here instead
// of corrupting the on-disk plan and breaking every subsequent Load.
func (s *PlanStore) WriteIssue(slug string, issue *schema.IssueYaml) error {
	dir := filepath.Join(s.root, slug, "issues")
	if err := s.mkdir(dir, 0o755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%d-%s.yaml", issue.ID, issue.Slug)
	data, err := yaml.Marshal(issue)
	if err != nil {
		return fmt.Errorf("marshaling issue: %w", err)
	}
	var probe schema.IssueYaml
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("issue %d-%s round-trip check: %w", issue.ID, issue.Slug, err)
	}
	return s.write(filepath.Join(dir, filename), data, 0o644)
}

// FindIssueFile finds the filename for an issue by ID prefix in a project.
func (s *PlanStore) FindIssueFile(slug string, id int) (string, error) {
	issuesDir := filepath.Join(slug, "issues")
	entries, err := fs.ReadDir(s.fsys, issuesDir)
	if err != nil {
		return "", fmt.Errorf("listing issues: %w", err)
	}

	prefix := fmt.Sprintf("%d-", id)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return filepath.Join(issuesDir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("issue #%d not found in %s", id, issuesDir)
}
