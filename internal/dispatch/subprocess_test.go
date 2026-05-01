package dispatch

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

const stubIssueYAML = `id: 5
slug: ship-it
name: Ship it
track: intent
status: in-progress
priority: high
points: 2
labels: [AFK]
blocked_by: []
blocking: []
created: "2026-04-30"
updated: "2026-04-30"
tdd: true
outcome: It ships
scope: Small
acceptance_criteria: ["It ships"]
steps: ["Target — ships"]
use_cases: ["UC-1"]
`

func writeStubIssue(t *testing.T, plansDir, slug, status string) {
	t.Helper()
	dir := filepath.Join(plansDir, slug, "issues")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	body := stubIssueYAML
	if status != "" {
		body = strings.Replace(body, "status: in-progress", "status: "+status, 1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "5-ship-it.yaml"), []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, slug, "prd.yaml"), []byte("name: Ship\nslug: ship\nstatus: active\n"), 0o644))
}

// installFakeClaude writes a shell script named `claude` to a fresh dir, prepends
// that dir to PATH, and returns the dir. The script body becomes the subprocess
// behavior. The cleanup restores PATH.
func installFakeClaude(t *testing.T, body string) string {
	t.Helper()
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "claude")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\n"+body), 0o755))

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)
	return binDir
}

func loadIssueFromDisk(t *testing.T, plansDir, slug string, id int) schema.IssueYaml {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(plansDir, slug, "issues", "5-ship-it.yaml"))
	require.NoError(t, err)
	var issue schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &issue))
	return issue
}

func TestRunSubprocess_SuccessFlipsToInReview(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writeStubIssue(t, plansDir, "ship", "")

	// Fake claude flips status in-review (mimicking the sub-agent calling pba complete).
	issuePath := filepath.Join(plansDir, "ship", "issues", "5-ship-it.yaml")
	body := `echo '{"type":"text","text":"working"}'
sed -i.bak 's/status: in-progress/status: in-review/' "` + issuePath + `"
echo '{"type":"text","text":"done"}'
exit 0
`
	installFakeClaude(t, body)

	worktree := t.TempDir()
	logDir := filepath.Join(t.TempDir(), "logs")
	var out bytes.Buffer

	issue := schema.IssueYaml{ID: 5, Slug: "ship-it", Status: "in-progress"}
	res := RunSubprocess(context.Background(), "ship", issue,
		"some prompt", worktree, plansDir, logDir, &out)

	require.True(t, res.Success, "expected success, got err: %v, out: %s", res.Err, out.String())
	assert.Contains(t, out.String(), "[issue-5] ")
	assert.Contains(t, out.String(), "working")

	post := loadIssueFromDisk(t, plansDir, "ship", 5)
	assert.Equal(t, "in-review", post.Status)

	logBytes, err := os.ReadFile(filepath.Join(logDir, "5.log"))
	require.NoError(t, err)
	assert.Contains(t, string(logBytes), "working")
}

func TestRunSubprocess_FailureMarksBlocked(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writeStubIssue(t, plansDir, "ship", "")

	body := `echo "boom" >&2
exit 1
`
	installFakeClaude(t, body)

	worktree := t.TempDir()
	logDir := filepath.Join(t.TempDir(), "logs")
	var out bytes.Buffer

	issue := schema.IssueYaml{ID: 5, Slug: "ship-it", Status: "in-progress"}
	res := RunSubprocess(context.Background(), "ship", issue,
		"some prompt", worktree, plansDir, logDir, &out)

	require.False(t, res.Success)
	require.Error(t, res.Err)

	post := loadIssueFromDisk(t, plansDir, "ship", 5)
	assert.Equal(t, "blocked", post.Status)
	require.NotNil(t, post.Notes)
	assert.Contains(t, *post.Notes, "boom")
}

func TestRunSubprocess_ExitZeroButStatusNotInReviewMarksBlocked(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writeStubIssue(t, plansDir, "ship", "")

	// Subprocess exits 0 but never flips status — common mistake the dispatch must catch.
	body := `echo "did some work but forgot pba complete"
exit 0
`
	installFakeClaude(t, body)

	worktree := t.TempDir()
	logDir := filepath.Join(t.TempDir(), "logs")
	var out bytes.Buffer

	issue := schema.IssueYaml{ID: 5, Slug: "ship-it", Status: "in-progress"}
	res := RunSubprocess(context.Background(), "ship", issue,
		"some prompt", worktree, plansDir, logDir, &out)

	require.False(t, res.Success)
	post := loadIssueFromDisk(t, plansDir, "ship", 5)
	assert.Equal(t, "blocked", post.Status)
	require.NotNil(t, post.Notes)
	assert.Contains(t, *post.Notes, "in-progress")
}

func TestRunSubprocess_MissingClaudeBinaryIsActionable(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	writeStubIssue(t, plansDir, "ship", "")

	// Empty PATH so claude is definitely not found.
	t.Setenv("PATH", "")

	worktree := t.TempDir()
	var out bytes.Buffer
	issue := schema.IssueYaml{ID: 5, Slug: "ship-it", Status: "in-progress"}
	res := RunSubprocess(context.Background(), "ship", issue,
		"prompt", worktree, plansDir, "", &out)

	require.False(t, res.Success)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "claude")
}

func TestBuildPrompt_ConcatenatesSkillAndIssue(t *testing.T) {
	worktree := t.TempDir()
	skillDir := filepath.Join(worktree, ".claude", "skills", "bender-implement-issue")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("# Implement Issue\n\nDo the thing."), 0o644))

	issue := schema.IssueYaml{ID: 7, Slug: "do-thing", Name: "Do the thing"}
	prompt, err := BuildPrompt(worktree, issue)
	require.NoError(t, err)

	assert.Contains(t, prompt, "Do the thing.")
	assert.Contains(t, prompt, "## Issue")
	assert.Contains(t, prompt, "id: 7")
	assert.Contains(t, prompt, "slug: do-thing")
}

func TestBuildPrompt_MissingSkillFileReturnsError(t *testing.T) {
	worktree := t.TempDir()
	issue := schema.IssueYaml{ID: 1, Slug: "x"}
	_, err := BuildPrompt(worktree, issue)
	require.Error(t, err)
}
