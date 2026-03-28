package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func validPrd() PrdYaml {
	return PrdYaml{
		Name:        "Test",
		Slug:        "test",
		Status:      "active",
		Created:     "2026-03-26",
		Updated:     "2026-03-26",
		Description: "A test PRD",
		Why:         "Because testing",
		Outcome:     "Better tests",
	}
}

func TestPrdValidate_Valid(t *testing.T) {
	prd := validPrd()
	errs := prd.Validate()
	assert.Empty(t, errs)
}

func TestPrdValidate_MissingRequired(t *testing.T) {
	prd := PrdYaml{Slug: "x"}
	errs := prd.Validate()
	assert.NotEmpty(t, errs)
	assertHasFieldError(t, errs, "name")
	assertHasFieldError(t, errs, "status")
	assertHasFieldError(t, errs, "created")
	assertHasFieldError(t, errs, "description")
	assertHasFieldError(t, errs, "why")
	assertHasFieldError(t, errs, "outcome")
}

func TestPrdValidate_InvalidStatus(t *testing.T) {
	prd := validPrd()
	prd.Status = "bogus"
	errs := prd.Validate()
	assertHasFieldError(t, errs, "status")
}

func TestPrdValidate_InvalidDateFormat(t *testing.T) {
	prd := validPrd()
	prd.Created = "March 26"
	errs := prd.Validate()
	assertHasFieldError(t, errs, "created")
}

func TestPrdValidate_AllStatuses(t *testing.T) {
	for _, s := range []string{"draft", "active", "in-review", "approved", "complete", "archived"} {
		prd := validPrd()
		prd.Status = s
		errs := prd.Validate()
		assert.Empty(t, errs, "status %q should be valid", s)
	}
}

func TestPrdYaml_RoundTrip(t *testing.T) {
	prd := validPrd()
	data, err := yaml.Marshal(&prd)
	require.NoError(t, err)

	var parsed PrdYaml
	require.NoError(t, yaml.Unmarshal(data, &parsed))
	assert.Equal(t, prd, parsed)
}

func assertHasFieldError(t *testing.T, errs []ValidationError, field string) {
	t.Helper()
	for _, e := range errs {
		if e.Field == field {
			return
		}
	}
	t.Errorf("expected field error for %q, got: %v", field, errs)
}
