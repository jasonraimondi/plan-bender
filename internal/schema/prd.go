package schema

import "regexp"

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var validPrdStatuses = map[string]bool{
	"draft": true, "active": true, "in-review": true,
	"approved": true, "complete": true, "archived": true,
}

// UseCase is a PRD use case entry.
type UseCase struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// LinearRef holds Linear integration metadata on a PRD.
type LinearRef struct {
	ProjectID string `json:"project_id,omitempty"`
}

// PRD represents a PRD JSON file.
type PRD struct {
	Name          string     `json:"name"`
	Slug          string     `json:"slug"`
	Status        string     `json:"status"`
	Created       string     `json:"created"`
	Updated       string     `json:"updated"`
	Description   string     `json:"description"`
	Why           string     `json:"why"`
	Outcome       string     `json:"outcome"`
	InScope       []string   `json:"in_scope,omitempty"`
	OutOfScope    []string   `json:"out_of_scope,omitempty"`
	UseCases      []UseCase  `json:"use_cases,omitempty"`
	Decisions     []string   `json:"decisions,omitempty"`
	OpenQuestions []string   `json:"open_questions,omitempty"`
	Risks         []string   `json:"risks,omitempty"`
	Validation    []string   `json:"validation,omitempty"`
	Notes         *string    `json:"notes,omitempty"`
	DevCommand    *string    `json:"dev_command,omitempty"`
	BaseURL       *string    `json:"base_url,omitempty"`
	Linear        *LinearRef `json:"linear,omitempty"`
}

// Validate checks required fields, enum values, and date formats.
func (p *PRD) Validate() []ValidationError {
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
