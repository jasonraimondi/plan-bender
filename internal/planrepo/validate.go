package planrepo

import (
	"fmt"
	"path/filepath"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Validate runs full plan validation against the in-session snapshot. It
// reuses the schema package validators so the result shape matches what
// disk-based tooling produces, but never re-reads from disk — the snapshot
// is the source of truth for the lifetime of the session.
func (s *PlanSession) Validate(cfg config.Config) schema.PlanValidationResult {
	return validateSnapshot(s.snapshot, s.baselineFilenames, cfg)
}

// Validate is a one-shot convenience that opens a session for slug, validates
// the snapshot, and closes the session. Open failures (missing plan,
// malformed YAML, lock contention) are surfaced as PRD errors in the returned
// result so callers always receive a PlanValidationResult and never need to
// distinguish "couldn't load" from "loaded but invalid" by error type.
func (p *Plans) Validate(slug string, cfg config.Config) schema.PlanValidationResult {
	sess, err := p.Open(slug)
	if err != nil {
		return schema.PlanValidationResult{
			PRD:    schema.ValidationResult{File: filepath.Join(slug, "prd.yaml"), Errors: []string{err.Error()}},
			Issues: []schema.ValidationResult{},
			Valid:  false,
		}
	}
	defer func() { _ = sess.Close() }()
	return sess.Validate(cfg)
}

func validateSnapshot(snap *Snapshot, baselineFilenames map[int]string, cfg config.Config) schema.PlanValidationResult {
	prdPath := filepath.Join(snap.Slug, "prd.yaml")

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
	for _, ve := range schema.ValidateCrossRefs(&snap.PRD, snap.Issues) {
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
	return fmt.Sprintf("%d-%s.yaml", iss.ID, iss.Slug)
}
