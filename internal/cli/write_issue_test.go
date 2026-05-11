package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validIssueYAML = `{
  "id": 1,
  "slug": "do-the-thing",
  "name": "Do the thing",
  "track": "intent",
  "status": "backlog",
  "priority": "medium",
  "points": 2,
  "labels": [],
  "assignee": null,
  "blocked_by": [],
  "blocking": [],
  "branch": null,
  "pr": null,
  "linear_id": null,
  "created": "2026-03-26",
  "updated": "2026-03-26",
  "tdd": false,
  "outcome": "Something works",
  "scope": "Small change",
  "acceptance_criteria": ["It works"],
  "steps": ["Target — does the thing"],
  "use_cases": []
}`

const validPrdYAML = `{
  "name": "Test Plan",
  "slug": "test-plan",
  "status": "active",
  "created": "2026-03-26",
  "updated": "2026-03-26",
  "description": "A test",
  "why": "Because",
  "outcome": "Success"
}`

// seedPlan writes a valid PRD into the plans dir for slug. Issues written
// after this are committed against a snapshot whose PRD already validates,
// matching realistic write-prd → write-issue ordering. Goes through the
// session API rather than raw os.WriteFile so the seed exercises the same
// on-disk contract the production code expects (including any future
// loader-level checks added by planrepo).
func seedPlan(t *testing.T, root, slug string) {
	t.Helper()
	plansDir := filepath.Join(root, ".plan-bender", "plans")
	var prd schema.PRD
	require.NoError(t, json.Unmarshal([]byte(validPrdYAML), &prd))
	prd.Slug = slug

	sess, err := planrepo.NewProd(plansDir).OpenOrCreate(slug)
	require.NoError(t, err)
	require.NoError(t, sess.UpdatePrd(prd))
	require.NoError(t, sess.Commit(config.Defaults()))
	require.NoError(t, sess.Close())
}

func TestWriteIssue_ValidIssue(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	seedPlan(t, dir, "test-plan")

	inputFile := filepath.Join(dir, "issue.json")
	require.NoError(t, os.WriteFile(inputFile, []byte(validIssueYAML), 0o644))

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan", inputFile})
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.json"))
	assert.NoError(t, err)
}

func TestWriteIssue_StdinPipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	seedPlan(t, dir, "test-plan")

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan"})
	cmd.SetIn(strings.NewReader(validIssueYAML))
	var out strings.Builder
	cmd.SetOut(&out)
	require.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "wrote")
	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.json"))
	assert.NoError(t, err)
}

// TestWriteIssue_AcceptsForwardRefs is the bootstrap regression test:
// the very first issue in a plan must be writable even when its blocking
// edges point at issues that haven't been authored yet. Strict cross-ref
// validation still runs at `agent validate` time; the per-write path is
// lax so authors don't have to write all issues with empty deps and patch
// them later.
func TestWriteIssue_AcceptsForwardRefs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	seedPlan(t, dir, "test-plan")

	forwardRefIssue := strings.Replace(validIssueYAML, `"blocking": []`, `"blocking": [2, 3, 9]`, 1)

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan"})
	cmd.SetIn(strings.NewReader(forwardRefIssue))
	var out, errBuf strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	require.NoError(t, cmd.Execute(), "stderr: %s", errBuf.String())

	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.json"))
	assert.NoError(t, err)
}

// TestWriteIssue_UpdatesExisting exercises the upsert routing inside
// stageIssue: a second write to the same ID must succeed (UpdateIssue path)
// rather than failing on duplicate-ID. Slug rename also exercises the
// session's filename-rewrite-with-cleanup logic.
func TestWriteIssue_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	seedPlan(t, dir, "test-plan")

	cmd := NewWriteIssueCmd()
	cmd.SetArgs([]string{"test-plan"})
	cmd.SetIn(strings.NewReader(validIssueYAML))
	cmd.SetOut(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	updated := strings.Replace(validIssueYAML, `"slug": "do-the-thing"`, `"slug": "renamed"`, 1)
	updated = strings.Replace(updated, `"name": "Do the thing"`, `"name": "Renamed"`, 1)
	cmd2 := NewWriteIssueCmd()
	cmd2.SetArgs([]string{"test-plan"})
	cmd2.SetIn(strings.NewReader(updated))
	cmd2.SetOut(&strings.Builder{})
	require.NoError(t, cmd2.Execute())

	_, err := os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-renamed.json"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ".plan-bender", "plans", "test-plan", "issues", "1-do-the-thing.json"))
	assert.True(t, os.IsNotExist(err), "old slug filename must be cleaned up after rename")
}
