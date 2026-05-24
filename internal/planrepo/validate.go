package planrepo

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Validate runs full plan validation against the in-session snapshot. It
// reuses the schema package validators so the result shape matches what
// disk-based tooling produces, but never re-reads from disk — the snapshot
// is the source of truth for the lifetime of the session.
//
// Strict cross-ref mode: this is the canonical full-plan check, used by the
// `agent validate` path. Commit uses the lax variant; see validateSnapshot.
func (s *PlanSession) Validate(cfg config.Config) schema.PlanValidationResult {
	return validateSnapshot(s.snapshot, s.baselineFilenames, cfg, schema.CrossRefStrict)
}

// Validate is a one-shot convenience that opens a session for slug, validates
// it, and closes the session. Open failures (missing plan, malformed JSON)
// are surfaced as PRD errors in the returned result so callers always get the
// PlanValidationResult shape — matching the behavior of the prior disk-based
// schema.ValidatePlan path.
func (p *Plans) Validate(slug string, cfg config.Config) schema.PlanValidationResult {
	sess, err := p.Open(slug)
	if err != nil {
		return openErrorAsValidationResult(slug, err)
	}
	defer func() { _ = sess.Close() }()
	return sess.Validate(cfg)
}

// openErrorAsValidationResult shapes a single Open failure into the
// PlanValidationResult contract. Parse errors get attributed to the actual
// broken file (PRD or issue) so `agent validate` points editors at the right
// place; everything else is reported against the PRD path.
func openErrorAsValidationResult(slug string, err error) schema.PlanValidationResult {
	prdPath := filepath.Join(slug, "prd.json")
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		if parseErr.File == prdPath {
			return schema.PlanValidationResult{
				PRD:    schema.ValidationResult{File: prdPath, Errors: []string{err.Error()}},
				Issues: []schema.ValidationResult{},
				Valid:  false,
			}
		}
		return schema.PlanValidationResult{
			PRD:    schema.ValidationResult{File: prdPath, Errors: nil},
			Issues: []schema.ValidationResult{{File: parseErr.File, Errors: []string{err.Error()}}},
			Valid:  false,
		}
	}
	return schema.PlanValidationResult{
		PRD:    schema.ValidationResult{File: prdPath, Errors: []string{err.Error()}},
		Issues: []schema.ValidationResult{},
		Valid:  false,
	}
}

func validateSnapshot(snap *Snapshot, baselineFilenames map[int]string, cfg config.Config, crossRefMode schema.CrossRefMode) schema.PlanValidationResult {
	prdPath := filepath.Join(snap.Slug, "prd.json")

	var prdErrs []string
	for _, ve := range snap.PRD.Validate() {
		prdErrs = append(prdErrs, ve.String())
	}

	issueResults := make([]schema.ValidationResult, 0, len(snap.Issues))
	for i := range snap.Issues {
		iss := &snap.Issues[i]
		var errs []string
		for _, ve := range iss.Validate(cfg) {
			errs = append(errs, ve.String())
		}
		issueResults = append(issueResults, schema.ValidationResult{
			File:   issueFilePath(snap.Slug, iss, baselineFilenames),
			Errors: errs,
		})
	}

	var crossRef []string
	for _, ve := range schema.ValidateCrossRefs(&snap.PRD, snap.Issues, crossRefMode) {
		crossRef = append(crossRef, ve.String())
	}

	cycles := schema.DetectCycles(snap.Issues)

	hasErrors := len(prdErrs) > 0 || len(crossRef) > 0 || len(cycles) > 0
	for _, r := range issueResults {
		if len(r.Errors) > 0 {
			hasErrors = true
			break
		}
	}

	return schema.PlanValidationResult{
		PRD:      schema.ValidationResult{File: prdPath, Errors: prdErrs},
		Issues:   issueResults,
		CrossRef: crossRef,
		Cycles:   cycles,
		Valid:    !hasErrors,
	}
}

// validateTouched validates only the entities mutated in this session: the
// dirty PRD (if any) and each dirty issue's own field rules. It deliberately
// skips clean issues and the whole-plan cross-ref/cycle checks — a single
// status transition cannot introduce those errors, and a pre-existing invalid
// issue elsewhere must not block the write. Whole-plan consistency stays the
// job of `agent validate` (see PlanSession.Validate).
func (s *PlanSession) validateTouched(cfg config.Config) schema.PlanValidationResult {
	prdResult := schema.ValidationResult{File: filepath.Join(s.snapshot.Slug, "prd.json")}
	if s.dirtyPRD {
		for _, ve := range s.snapshot.PRD.Validate() {
			prdResult.Errors = append(prdResult.Errors, ve.String())
		}
	}

	var issueResults []schema.ValidationResult
	for i := range s.snapshot.Issues {
		iss := &s.snapshot.Issues[i]
		if !s.dirtyIssues[iss.ID] {
			continue
		}
		var errs []string
		for _, ve := range iss.Validate(cfg) {
			errs = append(errs, ve.String())
		}
		issueResults = append(issueResults, schema.ValidationResult{
			File:   issueFilePath(s.snapshot.Slug, iss, s.baselineFilenames),
			Errors: errs,
		})
	}

	hasErrors := len(prdResult.Errors) > 0
	for _, r := range issueResults {
		if len(r.Errors) > 0 {
			hasErrors = true
			break
		}
	}
	return schema.PlanValidationResult{
		PRD:    prdResult,
		Issues: issueResults,
		Valid:  !hasErrors,
	}
}

// issueFilePath returns the on-disk path to use in validation results: the
// baseline filename if the issue existed at Open time, otherwise the
// canonical filename a Commit would write.
func issueFilePath(slug string, iss *schema.Issue, baseline map[int]string) string {
	dir := filepath.Join(slug, "issues")
	if name, ok := baseline[iss.ID]; ok {
		return filepath.Join(dir, name)
	}
	return filepath.Join(dir, canonicalIssueFilename(iss))
}

func canonicalIssueFilename(iss *schema.Issue) string {
	return fmt.Sprintf("%d-%s.json", iss.ID, iss.Slug)
}
