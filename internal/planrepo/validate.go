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
// it, and closes the session. Open failures (missing plan, malformed YAML)
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

// issueFilePath returns the on-disk path to use in validation results: the
// baseline filename if the issue existed at Open time, otherwise the
// canonical filename a Commit would write.
func issueFilePath(slug string, iss *schema.IssueYaml, baseline map[int]string) string {
	dir := filepath.Join(slug, "issues")
	if name, ok := baseline[iss.ID]; ok {
		return filepath.Join(dir, name)
	}
	return filepath.Join(dir, canonicalIssueFilename(iss))
}

func canonicalIssueFilename(iss *schema.IssueYaml) string {
	return fmt.Sprintf("%d-%s.json", iss.ID, iss.Slug)
}
