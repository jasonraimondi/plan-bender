package schema

import "regexp"

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var validPrdStatuses = map[string]bool{
	"draft": true, "active": true, "in-review": true,
	"approved": true, "complete": true, "archived": true,
}

// UseCase is a PRD use case entry.
type UseCase struct {
	ID          string `yaml:"id"`
	Description string `yaml:"description"`
}

// LinearRef holds Linear integration metadata on a PRD.
type LinearRef struct {
	ProjectID string `yaml:"project_id,omitempty"`
}

// PrdYaml represents a PRD YAML file.
type PrdYaml struct {
	Name          string    `yaml:"name"`
	Slug          string    `yaml:"slug"`
	Status        string    `yaml:"status"`
	Created       string    `yaml:"created"`
	Updated       string    `yaml:"updated"`
	Description   string    `yaml:"description"`
	Why           string    `yaml:"why"`
	Outcome       string    `yaml:"outcome"`
	InScope       []string  `yaml:"in_scope,omitempty"`
	OutOfScope    []string  `yaml:"out_of_scope,omitempty"`
	UseCases      []UseCase `yaml:"use_cases,omitempty"`
	Decisions     []string  `yaml:"decisions,omitempty"`
	OpenQuestions []string  `yaml:"open_questions,omitempty"`
	Risks         []string  `yaml:"risks,omitempty"`
	Validation    []string  `yaml:"validation,omitempty"`
	Notes         *string   `yaml:"notes,omitempty"`
	DevCommand    *string   `yaml:"dev_command,omitempty"`
	BaseURL       *string   `yaml:"base_url,omitempty"`
	Linear        *LinearRef `yaml:"linear,omitempty"`
}

// Validate checks required fields, enum values, and date formats.
func (p *PrdYaml) Validate() []ValidationError {
	var errs []ValidationError

	if p.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "required non-empty string"})
	}
	if p.Slug == "" {
		errs = append(errs, ValidationError{Field: "slug", Message: "required non-empty string"})
	}
	if !validPrdStatuses[p.Status] {
		errs = append(errs, ValidationError{Field: "status", Message: "must be one of draft, active, in-review, approved, complete, archived"})
	}
	if !dateRe.MatchString(p.Created) {
		errs = append(errs, ValidationError{Field: "created", Message: "must be a YYYY-MM-DD date"})
	}
	if !dateRe.MatchString(p.Updated) {
		errs = append(errs, ValidationError{Field: "updated", Message: "must be a YYYY-MM-DD date"})
	}
	if p.Description == "" {
		errs = append(errs, ValidationError{Field: "description", Message: "required non-empty string"})
	}
	if p.Why == "" {
		errs = append(errs, ValidationError{Field: "why", Message: "required non-empty string"})
	}
	if p.Outcome == "" {
		errs = append(errs, ValidationError{Field: "outcome", Message: "required non-empty string"})
	}

	return errs
}
