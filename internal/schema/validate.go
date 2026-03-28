package schema

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"gopkg.in/yaml.v3"
)

// ValidationResult holds errors for a single file.
type ValidationResult struct {
	File   string
	Errors []string
}

// PlanValidationResult is the full validation output.
type PlanValidationResult struct {
	PRD      ValidationResult
	Issues   []ValidationResult
	CrossRef []string
	Cycles   []string
	Valid    bool
}

// ValidatePlan runs full validation on a plan directory.
func ValidatePlan(slug string, cfg config.Config, fsys fs.FS) PlanValidationResult {
	planDir := slug

	// Validate PRD
	prdPath := filepath.Join(planDir, "prd.yaml")
	prd, prdResult := validatePrdFile(fsys, prdPath)

	// Validate issues
	issuesDir := filepath.Join(planDir, "issues")
	issues, issueResults := validateIssueFiles(fsys, issuesDir, cfg)

	// Cross-reference checks
	var crossRef []string
	if prd != nil {
		for _, ve := range ValidateCrossRefs(prd, issues) {
			crossRef = append(crossRef, ve.String())
		}
	}

	// Cycle detection
	cycles := DetectCycles(issues)

	hasErrors := len(prdResult.Errors) > 0 ||
		len(crossRef) > 0 ||
		len(cycles) > 0
	for _, ir := range issueResults {
		if len(ir.Errors) > 0 {
			hasErrors = true
			break
		}
	}

	return PlanValidationResult{
		PRD:      prdResult,
		Issues:   issueResults,
		CrossRef: crossRef,
		Cycles:   cycles,
		Valid:    !hasErrors,
	}
}

func validatePrdFile(fsys fs.FS, path string) (*PrdYaml, ValidationResult) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, ValidationResult{File: path, Errors: []string{fmt.Sprintf("failed to read: %v", err)}}
	}

	var prd PrdYaml
	if err := yaml.Unmarshal(data, &prd); err != nil {
		return nil, ValidationResult{File: path, Errors: []string{fmt.Sprintf("invalid YAML: %v", err)}}
	}

	var errs []string
	for _, ve := range prd.Validate() {
		errs = append(errs, ve.String())
	}
	return &prd, ValidationResult{File: path, Errors: errs}
}

func validateIssueFiles(fsys fs.FS, dir string, cfg config.Config) ([]IssueYaml, []ValidationResult) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, []ValidationResult{{File: dir, Errors: []string{fmt.Sprintf("failed to list: %v", err)}}}
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var issues []IssueYaml
	var results []ValidationResult

	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			results = append(results, ValidationResult{File: path, Errors: []string{fmt.Sprintf("failed to read: %v", err)}})
			continue
		}

		var issue IssueYaml
		if err := yaml.Unmarshal(data, &issue); err != nil {
			results = append(results, ValidationResult{File: path, Errors: []string{fmt.Sprintf("invalid YAML: %v", err)}})
			continue
		}

		var errs []string
		for _, ve := range issue.Validate(cfg) {
			errs = append(errs, ve.String())
		}
		results = append(results, ValidationResult{File: path, Errors: errs})
		issues = append(issues, issue)
	}

	return issues, results
}
