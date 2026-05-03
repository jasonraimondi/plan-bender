package schema

// ValidationResult holds errors for a single file.
type ValidationResult struct {
	File   string   `json:"file"`
	Errors []string `json:"errors"`
}

// PlanValidationResult is the full validation output produced by
// planrepo.PlanSession.Validate. Disk reads happen inside planrepo so that the
// validation result reflects the same parsing path as runtime reads.
type PlanValidationResult struct {
	PRD      ValidationResult   `json:"prd"`
	Issues   []ValidationResult `json:"issues"`
	CrossRef []string           `json:"cross_ref"`
	Cycles   []string           `json:"cycles"`
	Valid    bool               `json:"valid"`
}
