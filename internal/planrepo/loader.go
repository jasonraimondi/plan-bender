package planrepo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// ParseError wraps a JSON decode failure with the offending file path and the
// 1-based line number derived from the decoder's byte offset (0 when none was
// available). The CLI surfaces these as an INVALID_PLAN structured error
// rather than letting the raw decoder message leak as an INTERNAL fault.
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

func newParseError(file string, data []byte, err error) *ParseError {
	return &ParseError{File: file, Line: extractJSONLine(data, err), Err: err}
}

// extractJSONLine returns the 1-based line containing the decoder's byte
// offset. SyntaxError exposes the offset directly; UnmarshalTypeError exposes
// it via the Offset field. Other errors fall back to line 0.
func extractJSONLine(data []byte, err error) int {
	if err == nil {
		return 0
	}
	var offset int64
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxErr):
		offset = syntaxErr.Offset
	case errors.As(err, &typeErr):
		offset = typeErr.Offset
	default:
		return 0
	}
	if offset <= 0 || int(offset) > len(data) {
		return 0
	}
	return bytes.Count(data[:offset], []byte{'\n'}) + 1
}

// StrictUnmarshal parses JSON into out with strict field checking. The default
// json.Unmarshal silently accepts unknown fields; DisallowUnknownFields rejects
// any field not declared on the target struct so malformed plan files surface
// as errors. Exported so CLI write commands stay symmetric with the loader —
// otherwise a typo'd field is silently dropped at write time and the file on
// disk silently omits it.
func StrictUnmarshal(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected data after top-level JSON value")
	}
	return nil
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
// (canonical filename derives from {id}-{slug}.json).
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

func loadPRD(fsys fs.FS, slug string) (*schema.PRD, error) {
	path := filepath.Join(slug, "prd.json")
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			yamlPath := filepath.Join(slug, "prd.yaml")
			if _, statErr := fs.Stat(fsys, yamlPath); statErr == nil {
				return nil, fmt.Errorf("found legacy %s but no prd.json — run 'pb migrate' to convert", yamlPath)
			}
		}
		return nil, fmt.Errorf("reading prd %s: %w", path, err)
	}
	var prd schema.PRD
	if err := StrictUnmarshal(data, &prd); err != nil {
		return nil, newParseError(path, data, err)
	}
	return &prd, nil
}

// loadIssues returns parsed issues alongside the on-disk filenames in the
// same order. Sessions need the filenames so a slug rename can replace the
// original file rather than orphaning it.
func loadIssues(fsys fs.FS, slug string) ([]schema.Issue, []string, error) {
	issuesDir := filepath.Join(slug, "issues")
	entries, err := fs.ReadDir(fsys, issuesDir)
	if err != nil {
		// A plan with a prd.json but no issues/ dir yet (a freshly written
		// PRD before decomposition) simply has no issues — not an error. Any
		// other read failure is real and propagates.
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("listing issues in %s: %w", issuesDir, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	issues := make([]schema.Issue, 0, len(names))
	for _, name := range names {
		path := filepath.Join(issuesDir, name)
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, nil, fmt.Errorf("reading issue %s: %w", path, err)
		}
		var issue schema.Issue
		if err := StrictUnmarshal(data, &issue); err != nil {
			return nil, nil, newParseError(path, data, err)
		}
		issues = append(issues, issue)
	}
	return issues, names, nil
}
