package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// dispatchFixture holds the artifacts a dispatcher test needs: a real git repo
// with an initial commit, a plans dir under .plan-bender/plans/, a worktree-side
// SKILL.md so BuildPrompt succeeds, and an env-resident fake claude on PATH.
type dispatchFixture struct {
	root     string
	plansDir string
}

func setupDispatch(t *testing.T) *dispatchFixture {
	t.Helper()

	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	// First commit so HEAD is valid for branch/worktree creation.
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# repo\n"), 0o644))
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	plansDir := filepath.Join(root, ".plan-bender", "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "demo", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "demo", "prd.yaml"),
		[]byte("name: Demo\nslug: demo\nstatus: active\n"), 0o644))

	return &dispatchFixture{root: root, plansDir: plansDir}
}

func writeIssue(t *testing.T, plansDir string, iss schema.IssueYaml) {
	t.Helper()
	data, err := yaml.Marshal(iss)
	require.NoError(t, err)
	path := filepath.Join(plansDir, "demo", "issues", fmt.Sprintf("%d-%s.yaml", iss.ID, iss.Slug))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func mkAFKIssue(id int, slug, status string, blockedBy ...int) schema.IssueYaml {
	return schema.IssueYaml{
		ID:                 id,
		Slug:               slug,
		Name:               slug,
		Track:              "intent",
		Status:             status,
		Priority:           "high",
		Points:             1,
		Labels:             []string{"AFK"},
		BlockedBy:          blockedBy,
		Created:            "2026-04-30",
		Updated:            "2026-04-30",
		Outcome:            "out",
		Scope:              "scope",
		AcceptanceCriteria: []string{"ok"},
		Steps:              []string{"do — it"},
		UseCases:           []string{"UC-1"},
	}
}

// installClaudeStub writes a script that, when invoked, lets the test inject
// per-issue behavior by reading the issue id from the prompt arg passed via -p.
// The script body is just shell — `caseBody` is sourced as the script body.
func installClaudeStub(t *testing.T, body string) {
	t.Helper()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\n"+body), 0o755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// installSkillFile writes a stub bender-implement-issue/SKILL.md into root so
// BuildPrompt finds it (it reads from {worktreePath}/.claude/skills/...).
func installSkillFile(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "skills", "bender-implement-issue")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# stub skill\n"), 0o644))
}

func newDispatcher(fix *dispatchFixture) *Dispatcher {
	return &Dispatcher{
		Config:   config.Defaults(),
		Root:     fix.root,
		PlansDir: fix.plansDir,
		Out:      &bytes.Buffer{},
	}
}

func loadIssueYAML(t *testing.T, plansDir string, id int, slug string) schema.IssueYaml {
	t.Helper()
	path := filepath.Join(plansDir, "demo", "issues", fmt.Sprintf("%d-%s.yaml", id, slug))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var iss schema.IssueYaml
	require.NoError(t, yaml.Unmarshal(data, &iss))
	return iss
}

func TestDispatcher_AllDoneShortCircuits(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "first", "done"))
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "second", "done"))

	// claude stub that fails if called — proves no subprocess spawned
	installClaudeStub(t, "echo 'should not be called'\nexit 99\n")

	d := newDispatcher(fix)
	err := d.Run(context.Background(), "demo")
	require.NoError(t, err)
}

func TestDispatcher_HITLOnlyExitsWithSentinelError(t *testing.T) {
	fix := setupDispatch(t)
	hitl := mkAFKIssue(1, "decide", "todo")
	hitl.Labels = []string{"HITL"}
	writeIssue(t, fix.plansDir, hitl)

	installClaudeStub(t, "exit 99\n")

	d := newDispatcher(fix)
	err := d.Run(context.Background(), "demo")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHITLOnly), "expected ErrHITLOnly, got %v", err)
}

func TestDispatcher_PartialFailureMergesSuccessOnly(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "beta", "todo"))
	installSkillFile(t, fix.root)

	// Stub claude reads the prompt for the issue id and either flips status
	// (success) or exits 1 (failure). We discriminate by the issue slug because
	// the prompt embeds it via BuildPrompt.
	plansDirAbs := fix.plansDir
	body := fmt.Sprintf(`set -e
prompt="$5"
case "$prompt" in
  *"slug: alpha"*)
    sed -i.bak 's/status: todo/status: in-review/' "%s/demo/issues/1-alpha.yaml"
    echo '{"text":"alpha done"}'
    exit 0
    ;;
  *"slug: beta"*)
    echo "beta failure" >&2
    exit 1
    ;;
esac
echo "unknown prompt"
exit 2
`, plansDirAbs)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 15*time.Second)
	// After alpha → done and beta → blocked, no AFK candidates remain and no
	// HITL issues are pending — Run reports stuck so the human can resolve.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck")

	alpha := loadIssueYAML(t, fix.plansDir, 1, "alpha")
	beta := loadIssueYAML(t, fix.plansDir, 2, "beta")

	assert.Equal(t, "done", alpha.Status, "alpha should be merged and flipped to done")
	assert.Equal(t, "blocked", beta.Status, "beta should be blocked after subprocess failure")
}

func TestReadyAFK_DispatcherIntegration_RespectsDependencyOrder(t *testing.T) {
	fix := setupDispatch(t)
	// 1 blocks 2: only 1 is ready.
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "first", "todo"))
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "second", "todo", 1))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt="$5"
case "$prompt" in
  *"slug: first"*)
    sed -i.bak 's/status: todo/status: in-review/' "%s/demo/issues/1-first.yaml"
    exit 0
    ;;
  *"slug: second"*)
    sed -i.bak 's/status: todo/status: in-review/' "%s/demo/issues/2-second.yaml"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	require.NoError(t, d.Run(context.Background(), "demo"))

	first := loadIssueYAML(t, fix.plansDir, 1, "first")
	second := loadIssueYAML(t, fix.plansDir, 2, "second")
	assert.Equal(t, "done", first.Status)
	assert.Equal(t, "done", second.Status)
}

func TestEnsureIntegrationBranch_DirectStrategyUsesDefault(t *testing.T) {
	fix := setupDispatch(t)
	cfg := config.Defaults()
	cfg.Pipeline.BranchStrategy = "direct"
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir}

	branch, err := d.ensureIntegrationBranch("demo")
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestEnsureIntegrationBranch_IntegrationStrategyCreatesUserSlugBranch(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix) // defaults branch_strategy = integration

	branch, err := d.ensureIntegrationBranch("demo")
	require.NoError(t, err)
	assert.Equal(t, "tester/demo", branch)

	// branch should exist in the repo
	out, err := exec.Command("git", "-C", fix.root, "branch", "--list", "tester/demo").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "tester/demo")
}

func TestDispatcher_StuckOnAllBlockedReturnsError(t *testing.T) {
	fix := setupDispatch(t)
	// blocker 1 is blocked status → 2 can never start
	blocker := mkAFKIssue(1, "ghost", "blocked")
	dependent := mkAFKIssue(2, "needsghost", "todo", 1)
	writeIssue(t, fix.plansDir, blocker)
	writeIssue(t, fix.plansDir, dependent)
	installClaudeStub(t, "exit 99\n")

	d := newDispatcher(fix)
	err := d.Run(context.Background(), "demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck")
}

// timeBoxRun cancels the context if Run hangs longer than the deadline.
// Keeps test failure messages useful instead of a CI timeout kill.
func timeBoxRun(t *testing.T, d *Dispatcher, slug string, deadline time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, slug) }()
	select {
	case err := <-done:
		return err
	case <-time.After(deadline + time.Second):
		t.Fatal("Run did not return within deadline")
		return nil
	}
}

func TestDispatcher_CompletesMultiIssueBatch(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "beta", "todo"))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt="$5"
case "$prompt" in
  *"slug: alpha"*)
    sed -i.bak 's/status: todo/status: in-review/' "%s/demo/issues/1-alpha.yaml"
    exit 0
    ;;
  *"slug: beta"*)
    sed -i.bak 's/status: todo/status: in-review/' "%s/demo/issues/2-beta.yaml"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 30*time.Second)
	require.NoError(t, err)

	alpha := loadIssueYAML(t, fix.plansDir, 1, "alpha")
	beta := loadIssueYAML(t, fix.plansDir, 2, "beta")
	assert.Equal(t, "done", alpha.Status)
	assert.Equal(t, "done", beta.Status)
}

// TestDispatcher_BuildPromptFailureMarksBlocked asserts that an early failure
// in runOne (here: no SKILL.md installed → BuildPrompt fails) flips the issue
// to blocked instead of leaving it in todo. Without this, the next outer-loop
// iteration would re-pick the issue and dispatch would spin forever.
func TestDispatcher_BuildPromptFailureMarksBlocked(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	// Note: deliberately NOT calling installSkillFile so BuildPrompt fails.

	installClaudeStub(t, "exit 0\n")

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck", "loop must terminate, not retry forever")

	alpha := loadIssueYAML(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "blocked", alpha.Status, "early-failure issue must be marked blocked")
	require.NotNil(t, alpha.Notes)
	assert.Contains(t, *alpha.Notes, "building prompt", "block reason should reference the failure")
}

// quiet a couple of vet/staticcheck unused imports on environments where we
// trim them — left in for explicit signaling.
var _ = strings.HasPrefix
