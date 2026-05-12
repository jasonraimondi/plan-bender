package schema

import (
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

var validPriorities = map[string]bool{
	"urgent": true, "high": true, "medium": true, "low": true,
}

// Issue represents an issue JSON file.
type Issue struct {
	ID                 int      `json:"id"`
	Slug               string   `json:"slug"`
	Name               string   `json:"name"`
	Track              string   `json:"track"`
	Status             string   `json:"status"`
	Priority           string   `json:"priority"`
	Points             int      `json:"points"`
	Labels             []string `json:"labels"`
	Assignee           *string  `json:"assignee"`
	BlockedBy          []int    `json:"blocked_by"`
	Blocking           []int    `json:"blocking"`
	Branch             *string  `json:"branch"`
	PR                 *string  `json:"pr"`
	LinearID           *string  `json:"linear_id"`
	LinearURL          string   `json:"linear_url,omitempty"`
	Created            string   `json:"created"`
	Updated            string   `json:"updated"`
	TDD                bool     `json:"tdd"`
	Headed             *bool    `json:"headed,omitempty"`
	Outcome            string   `json:"outcome"`
	Scope              string   `json:"scope"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Steps              []string `json:"steps"`
	UseCases           []string `json:"use_cases"`
	Notes              *string  `json:"notes,omitempty"`
}

// Validate checks structural rules and config-dependent rules.
func (i *Issue) Validate(cfg config.Config) []ValidationError {
	var errs []ValidationError

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

	if hasDuplicates(i.BlockedBy) {
		errs = append(errs, ValidationError{Field: "blocked_by", Message: "contains duplicates"})
	}
	if hasDuplicates(i.Blocking) {
		errs = append(errs, ValidationError{Field: "blocking", Message: "contains duplicates"})
	}

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

	errs = append(errs, i.validateCustomFields(cfg)...)

	return errs
}

func (i *Issue) validateCustomFields(cfg config.Config) []ValidationError {
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
