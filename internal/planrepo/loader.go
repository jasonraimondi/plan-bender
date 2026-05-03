package planrepo

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// strictUnmarshal parses YAML into out with strict field checking. yaml.v3's
// plain Unmarshal silently accepts garbage like "::not yaml::" as a mapping
// with one unknown key; KnownFields(true) rejects any field not declared on
// the target struct so malformed plan files surface as errors.
func strictUnmarshal(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

// loadSnapshot reads and parses one plan's PRD and issue files through fsys
// (rooted at plansDir). Issue order is the lexicographic sort of the issue
// filenames so two snapshots of the same on-disk state are byte-identical.
func loadSnapshot(fsys fs.FS, slug string) (*Snapshot, error) {
	prd, err := loadPRD(fsys, slug)
	if err != nil {
		return nil, err
	}
	issues, err := loadIssues(fsys, slug)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Slug: slug, PRD: *prd, Issues: issues}, nil
}

func loadPRD(fsys fs.FS, slug string) (*schema.PrdYaml, error) {
	path := filepath.Join(slug, "prd.yaml")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("reading prd %s: %w", path, err)
	}
	var prd schema.PrdYaml
	if err := strictUnmarshal(data, &prd); err != nil {
		return nil, fmt.Errorf("parsing prd %s: %w", path, err)
	}
	return &prd, nil
}

func loadIssues(fsys fs.FS, slug string) ([]schema.IssueYaml, error) {
	issuesDir := filepath.Join(slug, "issues")
	entries, err := fs.ReadDir(fsys, issuesDir)
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
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("reading issue %s: %w", path, err)
		}
		var issue schema.IssueYaml
		if err := strictUnmarshal(data, &issue); err != nil {
			return nil, fmt.Errorf("parsing issue %s: %w", path, err)
		}
		issues = append(issues, issue)
	}
	return issues, nil
}
