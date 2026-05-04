package planrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/require"
)

// mustValidPRD returns a valid PrdYaml struct with the given slug.
func mustValidPRD(slug string) schema.PrdYaml {
	return schema.PrdYaml{
		Name:        "Fresh Plan",
		Slug:        slug,
		Status:      "active",
		Created:     "2026-01-01",
		Updated:     "2026-01-02",
		Description: "Fresh PRD",
		Why:         "Tests",
		Outcome:     "Passes",
	}
}

// writePlan creates a plan directory with a PRD and zero or more issue files.
// Issue file names come from the map keys so tests can exercise ordering.
func writePlan(t *testing.T, plansDir, slug, prd string, issues map[string]string) {
	t.Helper()
	planDir := filepath.Join(plansDir, slug)
	issuesDir := filepath.Join(planDir, "issues")
	require.NoError(t, os.MkdirAll(issuesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "prd.yaml"), []byte(prd), 0o644))
	for name, body := range issues {
		require.NoError(t, os.WriteFile(filepath.Join(issuesDir, name), []byte(body), 0o644))
	}
}

const validPrd = `name: Test Plan
slug: test-plan
status: active
created: "2026-01-01"
updated: "2026-01-02"
description: A test
why: Testing
outcome: Tests pass
`

func issueYAML(id int, slug string) string {
	return fmt.Sprintf(`id: %d
slug: %s
name: Issue %s
track: data
status: todo
priority: high
points: 1
labels: []
blocked_by: []
blocking: []
created: "2026-01-01"
updated: "2026-01-02"
outcome: ok
scope: small
acceptance_criteria: []
steps: []
use_cases: []
`, id, slug, slug)
}
