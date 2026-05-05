package schema

import "fmt"

// CrossRefMode controls how strictly ValidateCrossRefs treats blocked_by /
// blocking edges that point at issues which are not present in the snapshot.
//
// Strict: the canonical full-plan check. Missing targets and broken symmetry
// are errors. Used by `agent validate` and any caller that wants to confirm
// the plan as a whole is internally consistent.
//
// Lax: bootstrap-friendly. Edges whose target is absent are silently
// accepted (and their symmetry check is skipped, since there is nothing to
// be symmetric with). Edges between two present issues are still required
// to be symmetric. Used by the per-write commit path so the first issue in
// a plan can declare forward dependencies on issues that have not been
// written yet — a strict commit-time check would otherwise force authors to
// write all issues with empty deps and patch them after the fact.
type CrossRefMode int

const (
	CrossRefStrict CrossRefMode = iota
	CrossRefLax
)

// ValidateCrossRefs checks blocked_by/blocking references, symmetry, and
// use_case existence. See CrossRefMode for the strict/lax distinction.
// use_case validation is unaffected by mode — the PRD is always written
// before issues, so unknown use cases are always errors.
func ValidateCrossRefs(prd *PrdYaml, issues []IssueYaml, mode CrossRefMode) []ValidationError {
	var errs []ValidationError

	ids := make(map[int]bool, len(issues))
	for _, iss := range issues {
		ids[iss.ID] = true
	}

	blockedBySet := make(map[int]map[int]bool)
	blockingSet := make(map[int]map[int]bool)
	for _, iss := range issues {
		blockedBySet[iss.ID] = toSet(iss.BlockedBy)
		blockingSet[iss.ID] = toSet(iss.Blocking)
	}

	for _, iss := range issues {
		for _, dep := range iss.BlockedBy {
			if !ids[dep] {
				if mode == CrossRefStrict {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("issue #%d blocked_by", iss.ID),
						Message: fmt.Sprintf("references non-existent issue #%d", dep),
					})
				}
				continue
			}
			if !blockingSet[dep][iss.ID] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocked_by", iss.ID),
					Message: fmt.Sprintf("issue #%d does not list #%d in blocking", dep, iss.ID),
				})
			}
		}

		for _, dep := range iss.Blocking {
			if !ids[dep] {
				if mode == CrossRefStrict {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("issue #%d blocking", iss.ID),
						Message: fmt.Sprintf("references non-existent issue #%d", dep),
					})
				}
				continue
			}
			if !blockedBySet[dep][iss.ID] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocking", iss.ID),
					Message: fmt.Sprintf("issue #%d does not list #%d in blocked_by", dep, iss.ID),
				})
			}
		}

		if prd != nil {
			prdUCs := make(map[string]bool, len(prd.UseCases))
			for _, uc := range prd.UseCases {
				prdUCs[uc.ID] = true
			}
			for _, ucRef := range iss.UseCases {
				if !prdUCs[ucRef] {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("issue #%d use_cases", iss.ID),
						Message: fmt.Sprintf("references unknown use case %s", ucRef),
					})
				}
			}
		}
	}

	return errs
}

func toSet(ids []int) map[int]bool {
	s := make(map[int]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
