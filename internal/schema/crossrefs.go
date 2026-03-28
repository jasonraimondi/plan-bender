package schema

import "fmt"

// ValidateCrossRefs checks blocked_by/blocking references, symmetry, and use_case existence.
func ValidateCrossRefs(prd *PrdYaml, issues []IssueYaml) []ValidationError {
	var errs []ValidationError

	ids := make(map[int]bool, len(issues))
	for _, iss := range issues {
		ids[iss.ID] = true
	}

	// Build lookup for quick symmetry checks
	blockedBySet := make(map[int]map[int]bool)
	blockingSet := make(map[int]map[int]bool)
	for _, iss := range issues {
		blockedBySet[iss.ID] = toSet(iss.BlockedBy)
		blockingSet[iss.ID] = toSet(iss.Blocking)
	}

	for _, iss := range issues {
		// blocked_by targets must exist
		for _, dep := range iss.BlockedBy {
			if !ids[dep] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocked_by", iss.ID),
					Message: fmt.Sprintf("references non-existent issue #%d", dep),
				})
				continue
			}
			// Symmetry: dep.blocking should contain iss.ID
			if !blockingSet[dep][iss.ID] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocked_by", iss.ID),
					Message: fmt.Sprintf("issue #%d does not list #%d in blocking", dep, iss.ID),
				})
			}
		}

		// blocking targets must exist
		for _, dep := range iss.Blocking {
			if !ids[dep] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocking", iss.ID),
					Message: fmt.Sprintf("references non-existent issue #%d", dep),
				})
				continue
			}
			// Symmetry: dep.blocked_by should contain iss.ID
			if !blockedBySet[dep][iss.ID] {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("issue #%d blocking", iss.ID),
					Message: fmt.Sprintf("issue #%d does not list #%d in blocked_by", dep, iss.ID),
				})
			}
		}

		// use_case references must exist in PRD
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
