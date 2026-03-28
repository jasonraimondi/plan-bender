package schema

import (
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

var validPriorities = map[string]bool{
	"urgent": true, "high": true, "medium": true, "low": true,
}

// IssueYaml represents an issue YAML file.
type IssueYaml struct {
	ID                 int      `yaml:"id"`
	Slug               string   `yaml:"slug"`
	Name               string   `yaml:"name"`
	Track              string   `yaml:"track"`
	Status             string   `yaml:"status"`
	Priority           string   `yaml:"priority"`
	Points             int      `yaml:"points"`
	Labels             []string `yaml:"labels"`
	Assignee           *string  `yaml:"assignee"`
	BlockedBy          []int    `yaml:"blocked_by"`
	Blocking           []int    `yaml:"blocking"`
	Branch             *string  `yaml:"branch"`
	PR                 *string  `yaml:"pr"`
	LinearID           *string  `yaml:"linear_id"`
	LinearURL          string   `yaml:"linear_url,omitempty"`
	Created            string   `yaml:"created"`
	Updated            string   `yaml:"updated"`
	TDD                bool     `yaml:"tdd"`
	Headed             *bool    `yaml:"headed,omitempty"`
	Outcome            string   `yaml:"outcome"`
	Scope              string   `yaml:"scope"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	Steps              []string `yaml:"steps"`
	UseCases           []string `yaml:"use_cases"`
	Notes              *string  `yaml:"notes,omitempty"`
}

// Validate checks structural rules and config-dependent rules.
func (i *IssueYaml) Validate(cfg config.Config) []ValidationError {
	var errs []ValidationError

	// Required fields
	if i.Slug == "" {
		errs = append(errs, ValidationError{Field: "slug", Message: "required non-empty string"})
	}
	if i.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "required non-empty string"})
	}
	if i.Track == "" {
		errs = append(errs, ValidationError{Field: "track", Message: "required non-empty string"})
	}
	if i.Status == "" {
		errs = append(errs, ValidationError{Field: "status", Message: "required non-empty string"})
	}
	if i.Priority == "" {
		errs = append(errs, ValidationError{Field: "priority", Message: "required non-empty string"})
	}
	if i.Points < 1 {
		errs = append(errs, ValidationError{Field: "points", Message: "must be at least 1"})
	}
	if !dateRe.MatchString(i.Created) {
		errs = append(errs, ValidationError{Field: "created", Message: "must be a YYYY-MM-DD date"})
	}
	if !dateRe.MatchString(i.Updated) {
		errs = append(errs, ValidationError{Field: "updated", Message: "must be a YYYY-MM-DD date"})
	}
	if i.Outcome == "" {
		errs = append(errs, ValidationError{Field: "outcome", Message: "required non-empty string"})
	}
	if i.Scope == "" {
		errs = append(errs, ValidationError{Field: "scope", Message: "required non-empty string"})
	}

	// Self-reference checks
	for _, b := range i.BlockedBy {
		if b == i.ID {
			errs = append(errs, ValidationError{Field: "blocked_by", Message: "cannot reference self"})
			break
		}
	}
	for _, b := range i.Blocking {
		if b == i.ID {
			errs = append(errs, ValidationError{Field: "blocking", Message: "cannot reference self"})
			break
		}
	}

	// Duplicate checks
	if hasDuplicates(i.BlockedBy) {
		errs = append(errs, ValidationError{Field: "blocked_by", Message: "contains duplicates"})
	}
	if hasDuplicates(i.Blocking) {
		errs = append(errs, ValidationError{Field: "blocking", Message: "contains duplicates"})
	}

	// Config-dependent validation
	if i.Track != "" && !contains(cfg.Tracks, i.Track) {
		errs = append(errs, ValidationError{
			Field:   "track",
			Message: fmt.Sprintf("must be one of %v, got %q", cfg.Tracks, i.Track),
		})
	}
	if i.Status != "" && !contains(cfg.WorkflowStates, i.Status) {
		errs = append(errs, ValidationError{
			Field:   "status",
			Message: fmt.Sprintf("must be one of %v, got %q", cfg.WorkflowStates, i.Status),
		})
	}
	if !validPriorities[i.Priority] {
		errs = append(errs, ValidationError{
			Field:   "priority",
			Message: fmt.Sprintf("must be one of urgent, high, medium, low, got %q", i.Priority),
		})
	}
	if i.Points > cfg.MaxPoints {
		errs = append(errs, ValidationError{
			Field:   "points",
			Message: fmt.Sprintf("must be integer 1-%d, got %d", cfg.MaxPoints, i.Points),
		})
	}

	// Custom fields validation
	errs = append(errs, i.validateCustomFields(cfg)...)

	return errs
}

func (i *IssueYaml) validateCustomFields(cfg config.Config) []ValidationError {
	// Custom fields are not stored in the typed struct — they would need
	// to be accessed via a separate raw map. For now this is a placeholder
	// that will be wired when the backend reads raw YAML with extra fields.
	return nil
}

func hasDuplicates(ids []int) bool {
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
