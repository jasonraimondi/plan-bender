package planrepo

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"gopkg.in/yaml.v3"
)

// ParseError wraps a YAML decode failure with the offending file path and the
// first line number reported by the decoder (0 when none was reported). The
// CLI surfaces these as an INVALID_PLAN structured error rather than letting
// the raw yaml.v3 message leak as an INTERNAL fault.
type ParseError struct {
	File string
	Line int
	Err  error
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("parsing %s:%d: %s", e.File, e.Line, e.Err)
	}
	return fmt.Sprintf("parsing %s: %s", e.File, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// yamlLineRe matches "line N:" or "yaml: line N:" prefixes that yaml.v3
// embeds in TypeError messages and parse errors. We pull the first line
// number out so tooling can present a file:line pointer.
var yamlLineRe = regexp.MustCompile(`line (\d+):`)

func newParseError(file string, err error) *ParseError {
	return &ParseError{File: file, Line: extractYAMLLine(err), Err: err}
}

// extractYAMLLine returns the first line number embedded in err.Error(), or 0.
// yaml.v3 reports line numbers via the message string for both yaml.TypeError
// (one line per Errors entry) and other parse failures.
func extractYAMLLine(err error) int {
	if err == nil {
		return 0
	}
	if te, ok := err.(*yaml.TypeError); ok {
		for _, msg := range te.Errors {
			if n := matchLine(msg); n > 0 {
				return n
			}
		}
		return 0
	}
	return matchLine(err.Error())
}

func matchLine(s string) int {
	m := yamlLineRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

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
	snap, _, err := loadSnapshotWithFilenames(fsys, slug)
	return snap, err
}

// loadSnapshotWithFilenames also returns a map from issue ID to original
// on-disk filename. Sessions use this to detect slug renames at commit time
// (canonical filename derives from {id}-{slug}.yaml).
func loadSnapshotWithFilenames(fsys fs.FS, slug string) (*Snapshot, map[int]string, error) {
	prd, err := loadPRD(fsys, slug)
	if err != nil {
		return nil, nil, err
	}
	issues, names, err := loadIssues(fsys, slug)
	if err != nil {
		return nil, nil, err
	}
	filenames := make(map[int]string, len(issues))
	for i, iss := range issues {
		filenames[iss.ID] = names[i]
	}
	return &Snapshot{Slug: slug, PRD: *prd, Issues: issues}, filenames, nil
}

func loadPRD(fsys fs.FS, slug string) (*schema.PrdYaml, error) {
	path := filepath.Join(slug, "prd.yaml")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("reading prd %s: %w", path, err)
	}
	var prd schema.PrdYaml
	if err := strictUnmarshal(data, &prd); err != nil {
		return nil, newParseError(path, err)
	}
	return &prd, nil
}

// loadIssues returns parsed issues alongside the on-disk filenames in the
// same order. Sessions need the filenames so a slug rename can replace the
// original file rather than orphaning it.
func loadIssues(fsys fs.FS, slug string) ([]schema.IssueYaml, []string, error) {
	issuesDir := filepath.Join(slug, "issues")
	entries, err := fs.ReadDir(fsys, issuesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("listing issues in %s: %w", issuesDir, err)
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
			return nil, nil, fmt.Errorf("reading issue %s: %w", path, err)
		}
		var issue schema.IssueYaml
		if err := strictUnmarshal(data, &issue); err != nil {
			return nil, nil, newParseError(path, err)
		}
		issues = append(issues, issue)
	}
	return issues, names, nil
}
