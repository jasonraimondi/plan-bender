package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/worktree"
)

// dispatcherTestPrd is the PRD body written by setupDispatch. It is fully
// populated — including UC-1 in use_cases so cross-ref validation accepts
// the issues produced by mkAFKIssue — so planrepo.Commit's preflight
// validation accepts status writes from the prod owner adapter.
const dispatcherTestPrd = `{
  "name": "Demo",
  "slug": "demo",
  "status": "active",
  "created": "2026-04-30",
  "updated": "2026-04-30",
  "description": "demo plan",
  "why": "testing",
  "outcome": "demoed",
  "use_cases": [
    {"id": "UC-1", "description": "demo use case"}
  ]
}`

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
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "demo", "prd.json"),
		[]byte(dispatcherTestPrd), 0o644))

	return &dispatchFixture{root: root, plansDir: plansDir}
}

func writeIssue(t *testing.T, plansDir string, iss schema.Issue) {
	t.Helper()
	data, err := json.MarshalIndent(iss, "", "  ")
	require.NoError(t, err)
	path := filepath.Join(plansDir, "demo", "issues", fmt.Sprintf("%d-%s.json", iss.ID, iss.Slug))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func mkAFKIssue(id int, slug, status string, blockedBy ...int) schema.Issue {
	return schema.Issue{
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

func loadIssueJSON(t *testing.T, plansDir string, id int, slug string) schema.Issue {
	t.Helper()
	path := filepath.Join(plansDir, "demo", "issues", fmt.Sprintf("%d-%s.json", id, slug))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var iss schema.Issue
	require.NoError(t, json.Unmarshal(data, &iss))
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
prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    echo '{"text":"alpha done"}'
    exit 0
    ;;
  *"\"slug\": \"beta\""*)
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

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	beta := loadIssueJSON(t, fix.plansDir, 2, "beta")

	assert.Equal(t, "done", alpha.Status, "alpha should be merged and flipped to done")
	assert.Equal(t, "blocked", beta.Status, "beta should be blocked after subprocess failure")
}

func TestReadyAFK_DispatcherIntegration_RespectsDependencyOrder(t *testing.T) {
	fix := setupDispatch(t)
	// 1 blocks 2: only 1 is ready.
	first := mkAFKIssue(1, "first", "todo")
	first.Blocking = []int{2}
	writeIssue(t, fix.plansDir, first)
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "second", "todo", 1))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"first\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-first.json"
    exit 0
    ;;
  *"\"slug\": \"second\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/2-second.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	require.NoError(t, d.Run(context.Background(), "demo"))

	firstResult := loadIssueJSON(t, fix.plansDir, 1, "first")
	secondResult := loadIssueJSON(t, fix.plansDir, 2, "second")
	assert.Equal(t, "done", firstResult.Status)
	assert.Equal(t, "done", secondResult.Status)
}

func TestEnsureIntegrationBranch_DirectStrategyUsesDefault(t *testing.T) {
	fix := setupDispatch(t)
	cfg := config.Defaults()
	cfg.Pipeline.BranchStrategy = "direct"
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir}

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "main", branch)
}

func TestEnsureIntegrationBranch_IntegrationStrategyCreatesUserSlugBranch(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "tester/demo", branch)

	out, err := exec.Command("git", "-C", fix.root, "branch", "--list", "tester/demo").Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "tester/demo")
}

func TestEnsureIntegrationBranch_BaseOverridesDefault_Integration(t *testing.T) {
	fix := setupDispatch(t)

	// Create a "feature-x" branch one commit ahead of main so a fork off
	// feature-x produces a different SHA than a fork off main.
	for _, args := range [][]string{
		{"checkout", "-b", "feature-x"},
		{"commit", "--allow-empty", "-m", "feature-x commit"},
		{"checkout", "main"},
	} {
		out, err := exec.Command("git", append([]string{"-C", fix.root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	d := newDispatcher(fix)
	d.Base = "feature-x"

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "tester/demo", branch)

	integrationSHA, err := exec.Command("git", "-C", fix.root, "rev-parse", "tester/demo").Output()
	require.NoError(t, err)
	featureSHA, err := exec.Command("git", "-C", fix.root, "rev-parse", "feature-x").Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(featureSHA)), strings.TrimSpace(string(integrationSHA)))
}

func TestEnsureIntegrationBranch_BaseOverridesDefault_Direct(t *testing.T) {
	fix := setupDispatch(t)

	out, err := exec.Command("git", "-C", fix.root, "checkout", "-b", "feature-x").CombinedOutput()
	require.NoError(t, err, "checkout: %s", string(out))
	out, err = exec.Command("git", "-C", fix.root, "checkout", "main").CombinedOutput()
	require.NoError(t, err, "checkout main: %s", string(out))

	cfg := config.Defaults()
	cfg.Pipeline.BranchStrategy = "direct"
	d := &Dispatcher{Config: cfg, Root: fix.root, PlansDir: fix.plansDir, Base: "feature-x"}

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "feature-x", branch, "direct strategy must return --base as merge target")
}

func TestEnsureIntegrationBranch_BaseAcceptsSHA(t *testing.T) {
	fix := setupDispatch(t)

	shaBytes, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(shaBytes))

	d := newDispatcher(fix)
	d.Base = sha

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "tester/demo", branch)

	integrationSHA, err := exec.Command("git", "-C", fix.root, "rev-parse", "tester/demo").Output()
	require.NoError(t, err)
	assert.Equal(t, sha, strings.TrimSpace(string(integrationSHA)))
}

func TestEnsureIntegrationBranch_ExistingBranchWithBaseWarns(t *testing.T) {
	fix := setupDispatch(t)

	// Pre-create tester/demo so ensureIntegrationBranch hits the reuse path.
	out, err := exec.Command("git", "-C", fix.root, "branch", "tester/demo").CombinedOutput()
	require.NoError(t, err, "pre-create branch: %s", string(out))

	buf := &bytes.Buffer{}
	d := &Dispatcher{
		Config:   config.Defaults(),
		Root:     fix.root,
		PlansDir: fix.plansDir,
		Out:      buf,
		Base:     "main",
	}

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "tester/demo", branch)
	assert.Contains(t, buf.String(), "tester/demo")
	assert.Contains(t, buf.String(), "--base")
	assert.Contains(t, buf.String(), "ignored")
}

func TestEnsureIntegrationBranch_ExistingBranchWithoutBaseSilent(t *testing.T) {
	fix := setupDispatch(t)

	out, err := exec.Command("git", "-C", fix.root, "branch", "tester/demo").CombinedOutput()
	require.NoError(t, err, "pre-create branch: %s", string(out))

	buf := &bytes.Buffer{}
	d := &Dispatcher{Config: config.Defaults(), Root: fix.root, PlansDir: fix.plansDir, Out: buf}

	_, err = d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "ignored", "no --base = no warning")
}

func TestEnsureIntegrationBranch_BaseBypassesDetachedHEADError(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))
	for _, args := range [][]string{
		{"init", "--initial-branch=feature-only"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# r\n"), 0o644))
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	out, err := exec.Command("git", "-C", root, "checkout", "--detach", "HEAD").CombinedOutput()
	require.NoError(t, err, "detach: %s", string(out))

	plansDir := filepath.Join(root, ".plan-bender", "plans")
	require.NoError(t, os.MkdirAll(filepath.Join(plansDir, "demo", "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "demo", "prd.json"),
		[]byte(dispatcherTestPrd), 0o644))

	d := &Dispatcher{
		Config:   config.Defaults(),
		Root:     root,
		PlansDir: plansDir,
		Out:      &bytes.Buffer{},
		Base:     "feature-only",
	}

	branch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err, "explicit --base must bypass detached-HEAD detection")
	assert.Equal(t, "tester/demo", branch)
}

// TestDefaultBranch_RejectsDetachedHEAD ensures defaultBranch errors out instead
// of returning the literal "HEAD" when the repo is in detached state and has no
// main/master and no origin/HEAD — otherwise downstream `git branch user/slug
// HEAD` would create a branch literally named off "HEAD", or worse, succeed
// silently and leave the integration branch named "HEAD".
func TestDefaultBranch_RejectsDetachedHEAD(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	for _, args := range [][]string{
		{"init", "--initial-branch=feature-only"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Test User"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# repo\n"), 0o644))
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "init"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	// Detach HEAD on the only commit.
	out, err := exec.Command("git", "-C", root, "checkout", "--detach", "HEAD").CombinedOutput()
	require.NoError(t, err, "detach: %s", string(out))

	branch, err := defaultBranch(context.Background(), root)
	require.Error(t, err, "must reject detached HEAD instead of returning literal \"HEAD\"")
	assert.NotEqual(t, "HEAD", branch)
}

func TestDispatcher_StuckOnAllBlockedReturnsError(t *testing.T) {
	fix := setupDispatch(t)
	blocker := mkAFKIssue(1, "ghost", "blocked")
	dependent := mkAFKIssue(2, "needsghost", "todo", 1)
	writeIssue(t, fix.plansDir, blocker)
	writeIssue(t, fix.plansDir, dependent)
	installClaudeStub(t, "exit 99\n")

	d := newDispatcher(fix)
	err := d.Run(context.Background(), "demo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck")
	// Issue 1 is blocked with no open dependency blockers — the operationally
	// blocked case. The stuck message must count it (and name it), not report
	// the misleading "0 blocked" that plan.Resolve's BlockedCount would yield.
	assert.Contains(t, err.Error(), "1 blocked")
	assert.Contains(t, err.Error(), "#1")
}

// TestDispatcher_RunBatchRespectsMaxParallelCap dispatches more ready issues
// than the configured max_parallel and asserts no more than that many claude
// subprocesses ever run at once. Each stub drops a marker file while running;
// every invocation samples the live marker count, and the peak sample must
// stay at or below the cap.
func TestDispatcher_RunBatchRespectsMaxParallelCap(t *testing.T) {
	fix := setupDispatch(t)
	installSkillFile(t, fix.root)

	const numIssues = 5
	const maxPar = 2
	for i := 1; i <= numIssues; i++ {
		writeIssue(t, fix.plansDir, mkAFKIssue(i, fmt.Sprintf("iss%d", i), "todo"))
	}

	countDir := t.TempDir()
	samplesFile := filepath.Join(t.TempDir(), "samples")
	t.Setenv("PB_COUNTDIR", countDir)
	t.Setenv("PB_SAMPLES", samplesFile)

	var cases strings.Builder
	for i := 1; i <= numIssues; i++ {
		fmt.Fprintf(&cases, "  *'\"slug\": \"iss%d\"'*) target=\"%s/demo/issues/%d-iss%d.json\" ;;\n",
			i, fix.plansDir, i, i)
	}

	// Marker-file dance: create a uniquely-named marker, sample how many markers
	// are live (= concurrent subprocesses), sleep to force overlap, then clear
	// the marker and flip the issue to in-review.
	body := fmt.Sprintf(`prompt=$(cat)
mine=$(mktemp "$PB_COUNTDIR/run.XXXXXX")
ls "$PB_COUNTDIR" | wc -l | tr -d ' ' >> "$PB_SAMPLES"
sleep 0.4
rm -f "$mine"
target=""
case "$prompt" in
%sesac
sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "$target"
exit 0
`, cases.String())
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	limit := maxPar
	d.Config.Pipeline.MaxParallel = &limit
	require.NoError(t, timeBoxRun(t, d, "demo", 60*time.Second))

	data, err := os.ReadFile(samplesFile)
	require.NoError(t, err)
	peak := 0
	for _, field := range strings.Fields(string(data)) {
		n, convErr := strconv.Atoi(field)
		require.NoError(t, convErr)
		if n > peak {
			peak = n
		}
	}
	assert.LessOrEqual(t, peak, maxPar, "peak concurrent subprocesses must not exceed max_parallel")
	assert.GreaterOrEqual(t, peak, 2, "expected the batch to run subprocesses in parallel")
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

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    exit 0
    ;;
  *"\"slug\": \"beta\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/2-beta.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 30*time.Second)
	require.NoError(t, err)

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	beta := loadIssueJSON(t, fix.plansDir, 2, "beta")
	assert.Equal(t, "done", alpha.Status)
	assert.Equal(t, "done", beta.Status)
}

// TestDispatcher_RunOneClaimsBeforeSubprocess asserts dispatch atomically claims
// the issue (status=in-progress + branch stamped) BEFORE the sub-agent reads
// the issue JSON. Without this the implement-issue skill prompts the agent to set
// `branch:` and `status:` itself by textual edit, and a naive Edit produces
// duplicate keys that yaml.v3 then rejects on every subsequent Load. The
// regression we're guarding against is the v0.0.35 corruption where
// `dispatch popar-py` left issue YAML files with two `branch:` keys.
func TestDispatcher_RunOneClaimsBeforeSubprocess(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "backlog"))
	installSkillFile(t, fix.root)

	// Stub claude that asserts the prompt embeds in-progress + a non-null
	// branch (proving Claim already ran), then flips the issue to in-review.
	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *'"status": "in-progress"'*) ;;
  *) echo "expected in-progress in prompt, got:" >&2; echo "$prompt" >&2; exit 11 ;;
esac
case "$prompt" in
  *'"branch": "tester/demo--1-alpha"'*) ;;
  *) echo "expected branch stamped in prompt" >&2; exit 12 ;;
esac
sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
exit 0
`, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	require.NoError(t, timeBoxRun(t, d, "demo", 15*time.Second))

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", alpha.Status)
	require.NotNil(t, alpha.Branch, "Claim must stamp branch on the issue JSON")
	assert.Equal(t, "tester/demo--1-alpha", *alpha.Branch)
}

// TestDispatcher_BuildPromptFailureMarksBlocked asserts that an early failure
// in runOne (here: no SKILL.md installed → BuildPrompt fails) flips the issue
// to blocked instead of leaving it in todo. Without this, the next outer-loop
// iteration would re-pick the issue and dispatch would spin forever.
func TestDispatcher_BuildPromptFailureMarksBlocked(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))

	installClaudeStub(t, "exit 0\n")

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck", "loop must terminate, not retry forever")

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "blocked", alpha.Status, "early-failure issue must be marked blocked")
	require.NotNil(t, alpha.Notes)
	assert.Contains(t, *alpha.Notes, "building prompt", "block reason should reference the failure")
}

// TestDispatcher_ClaimFailureRemovesLeakedWorktree asserts that when runOne
// fails after worktree.Create but before the subprocess starts, the orphaned
// worktree is removed instead of left on disk. The Claim is forced to fail by
// seeding the issue in `done` — a status outside Claim's CAS from-set — so
// Create succeeds but Claim returns a mismatch.
func TestDispatcher_ClaimFailureRemovesLeakedWorktree(t *testing.T) {
	fix := setupDispatch(t)
	iss := mkAFKIssue(1, "alpha", "done")
	writeIssue(t, fix.plansDir, iss)

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	logDir := filepath.Join(fix.root, ".plan-bender", "logs", "demo")
	res := d.runOne(context.Background(), "demo", iss, logDir, integrationBranch)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "claiming issue")

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	wtPath := filepath.Join(parent, "repo-wt", "demo", "1-alpha")
	_, statErr := os.Stat(wtPath)
	assert.True(t, os.IsNotExist(statErr), "leaked worktree must be removed, still present at %s", wtPath)

	out, err := exec.Command("git", "-C", fix.root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	assert.NotContains(t, string(out), wtPath, "git must no longer track the removed worktree")
}

// TestDispatcher_BuildPromptFailureRemovesLeakedWorktree asserts the worktree
// is removed when BuildPrompt fails (no SKILL.md installed) — earlier than
// the Claim path but still post-worktree.Create. Without cleanup the next
// loop iteration would hit "worktree already exists" on every retry.
func TestDispatcher_BuildPromptFailureRemovesLeakedWorktree(t *testing.T) {
	fix := setupDispatch(t)
	iss := mkAFKIssue(1, "alpha", "todo")
	writeIssue(t, fix.plansDir, iss)
	// Intentionally no installSkillFile — BuildPrompt will fail on missing SKILL.md.

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	logDir := filepath.Join(fix.root, ".plan-bender", "logs", "demo")
	res := d.runOne(context.Background(), "demo", iss, logDir, integrationBranch)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "building prompt")

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	wtPath := filepath.Join(parent, "repo-wt", "demo", "1-alpha")
	_, statErr := os.Stat(wtPath)
	assert.True(t, os.IsNotExist(statErr), "leaked worktree must be removed, still present at %s", wtPath)
}

// TestDispatcher_BeforeIssueHookFailureRemovesLeakedWorktree asserts the
// worktree is removed when a configured before_issue hook fails. Same leak
// pattern as the Claim/BuildPrompt paths, different trigger.
func TestDispatcher_BeforeIssueHookFailureRemovesLeakedWorktree(t *testing.T) {
	fix := setupDispatch(t)
	iss := mkAFKIssue(1, "alpha", "todo")
	writeIssue(t, fix.plansDir, iss)
	installSkillFile(t, fix.root)

	d := newDispatcher(fix)
	d.Config.Hooks.BeforeIssue = "exit 1"
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	logDir := filepath.Join(fix.root, ".plan-bender", "logs", "demo")
	res := d.runOne(context.Background(), "demo", iss, logDir, integrationBranch)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "before_issue hook")

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	wtPath := filepath.Join(parent, "repo-wt", "demo", "1-alpha")
	_, statErr := os.Stat(wtPath)
	assert.True(t, os.IsNotExist(statErr), "leaked worktree must be removed, still present at %s", wtPath)
}

// TestDispatcher_LinkPlansDirFailureRemovesLeakedWorktree asserts the worktree
// is removed when linkPlansDir rejects a real .plan-bender dir already
// present in the checkout. Committing .plan-bender to the integration branch
// reproduces the rejection path inside runOne.
func TestDispatcher_LinkPlansDirFailureRemovesLeakedWorktree(t *testing.T) {
	fix := setupDispatch(t)
	// Commit a .plan-bender dir to main so the integration branch's checkout
	// contains it as a real path. linkPlansDir then errors instead of
	// symlinking, which is the failure path under test.
	keep := filepath.Join(fix.root, ".plan-bender", ".keep")
	require.NoError(t, os.WriteFile(keep, []byte(""), 0o644))
	for _, args := range [][]string{
		{"add", ".plan-bender/.keep"},
		{"commit", "-m", "commit .plan-bender"},
	} {
		out, err := exec.Command("git", append([]string{"-C", fix.root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}

	iss := mkAFKIssue(1, "alpha", "todo")
	writeIssue(t, fix.plansDir, iss)
	installSkillFile(t, fix.root)

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	logDir := filepath.Join(fix.root, ".plan-bender", "logs", "demo")
	res := d.runOne(context.Background(), "demo", iss, logDir, integrationBranch)
	require.Error(t, res.Err)
	assert.Contains(t, res.Err.Error(), "linking plans dir")

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	wtPath := filepath.Join(parent, "repo-wt", "demo", "1-alpha")
	_, statErr := os.Stat(wtPath)
	assert.True(t, os.IsNotExist(statErr), "leaked worktree must be removed, still present at %s", wtPath)
}

// TestDispatcher_MergeBackRecoversFromStaleMergeState asserts that when an
// integration worktree from a prior crashed run is left with MERGE_HEAD set
// and a dirty file in the tree, the next MergeBack resets the worktree clean
// and proceeds with the new merge. This is the foundational reset-on-entry
// contract — without it, every crashed dispatch would require manual cleanup.
func TestDispatcher_MergeBackRecoversFromStaleMergeState(t *testing.T) {
	fix := setupDispatch(t)
	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	branch := "tester/demo--1-alpha"
	makeMergeableBranch(t, fix.root, integrationBranch, branch, "alpha.txt")

	iss := mkAFKIssue(1, "alpha", "in-review")
	iss.Branch = &branch
	writeIssue(t, fix.plansDir, iss)
	installSkillFile(t, fix.root)

	// Pre-create the iwt and inject stale state, mimicking a crashed prior run.
	iwt, err := worktree.CreateIntegration(context.Background(), d.Root, d.Config, "demo")
	require.NoError(t, err)

	gitDirOut, err := exec.Command("git", "-C", iwt.Path, "rev-parse", "--git-dir").Output()
	require.NoError(t, err)
	gitDir := strings.TrimSpace(string(gitDirOut))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(iwt.Path, gitDir)
	}
	branchTip, err := exec.Command("git", "-C", fix.root, "rev-parse", branch).Output()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "MERGE_HEAD"), branchTip, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(iwt.Path, "garbage.txt"), []byte("crash leftover\n"), 0o644))

	// Stub fails the test if invoked — recovery must not spawn a sub-agent.
	installClaudeStub(t, "echo 'should not be called' >&2\nexit 99\n")

	require.NoError(t, timeBoxRun(t, d, "demo", 15*time.Second))

	_, statErr := os.Stat(filepath.Join(gitDir, "MERGE_HEAD"))
	assert.True(t, os.IsNotExist(statErr), "MERGE_HEAD must be cleared by ResetIntegration")
	_, statErr = os.Stat(filepath.Join(iwt.Path, "garbage.txt"))
	assert.True(t, os.IsNotExist(statErr), "untracked file must be cleaned by ResetIntegration")

	post := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", post.Status, "issue must reach done after the recovered merge")

	logOut, err := exec.Command("git", "-C", fix.root, "log", "--oneline", integrationBranch).CombinedOutput()
	require.NoError(t, err, "git log: %s", string(logOut))
	assert.Contains(t, string(logOut), "merge issue #1", "integration branch must carry the merge commit")
}

// TestDispatcher_CrossSlugParallelRuns asserts two Run calls on distinct slugs
// in the same parent repo complete without interfering, and the parent HEAD
// never moves. This is the headline acceptance criterion for the integration-
// worktree refactor — go test -race surfaces any unprotected shared state.
func TestDispatcher_CrossSlugParallelRuns(t *testing.T) {
	fix := setupDispatch(t)
	installSkillFile(t, fix.root)

	// Add a second plan "demo2" alongside the existing "demo".
	require.NoError(t, os.MkdirAll(filepath.Join(fix.plansDir, "demo2", "issues"), 0o755))
	demo2PRD := strings.Replace(strings.Replace(dispatcherTestPrd,
		`"slug": "demo"`, `"slug": "demo2"`, 1),
		`"name": "Demo"`, `"name": "Demo2"`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(fix.plansDir, "demo2", "prd.json"),
		[]byte(demo2PRD), 0o644))

	// One issue per plan. writeIssue hardcodes "demo" so demo2's issue is
	// written by hand here.
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	beta := mkAFKIssue(1, "beta", "todo")
	betaData, err := json.MarshalIndent(beta, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(fix.plansDir, "demo2", "issues", "1-beta.json"), betaData, 0o644))

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    exit 0
    ;;
  *"\"slug\": \"beta\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo2/issues/1-beta.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir, fix.plansDir)
	installClaudeStub(t, body)

	refBefore, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaBefore, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)

	d1 := newDispatcher(fix)
	d2 := newDispatcher(fix)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	errCh := make(chan error, 2)
	go func() { errCh <- d1.Run(ctx, "demo") }()
	go func() { errCh <- d2.Run(ctx, "demo2") }()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("parallel Run did not complete within deadline")
		}
	}

	refAfter, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaAfter, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(refBefore)), strings.TrimSpace(string(refAfter)),
		"parent symbolic ref must not move across parallel dispatches")
	assert.Equal(t, strings.TrimSpace(string(shaBefore)), strings.TrimSpace(string(shaAfter)),
		"parent HEAD commit must not move across parallel dispatches")

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", alpha.Status)

	betaPath := filepath.Join(fix.plansDir, "demo2", "issues", "1-beta.json")
	betaData, err = os.ReadFile(betaPath)
	require.NoError(t, err)
	var betaPost schema.Issue
	require.NoError(t, json.Unmarshal(betaData, &betaPost))
	assert.Equal(t, "done", betaPost.Status)
}

// TestDispatcher_AllDoneRemovesIntegrationWorktree asserts the final GC pass on
// the AllDone exit path tears down the per-slug integration worktree along with
// any issue worktrees. HITL-only and error exits preserve the iwt for
// resumption (covered by adjacent tests that exit non-zero).
func TestDispatcher_AllDoneRemovesIntegrationWorktree(t *testing.T) {
	fix := setupDispatch(t)
	// Real-world projects gitignore both .plan-bender and .claude so linkPlansDir's
	// symlinks register as ignored entries — without this, the per-batch
	// non-forcing `worktree remove` preserves issue worktrees and the slug dir
	// would never reach empty even after AllDone.
	// Patterns are slash-less because linkPlansDir installs symlinks (not real
	// directories) and gitignore's trailing-slash form only matches directories.
	require.NoError(t, os.WriteFile(filepath.Join(fix.root, ".gitignore"),
		[]byte(".plan-bender\n.claude\n"), 0o644))
	for _, args := range [][]string{
		{"add", ".gitignore"},
		{"commit", "-m", "gitignore"},
	} {
		out, err := exec.Command("git", append([]string{"-C", fix.root}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, string(out))
	}
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	require.NoError(t, timeBoxRun(t, d, "demo", 15*time.Second))

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	slugDir := filepath.Join(parent, "repo-wt", "demo")

	iwtPath := filepath.Join(slugDir, "_integration")
	_, statErr := os.Stat(iwtPath)
	assert.True(t, os.IsNotExist(statErr), "integration worktree must be removed at %s", iwtPath)

	out, err := exec.Command("git", "-C", fix.root, "worktree", "list", "--porcelain").Output()
	require.NoError(t, err)
	assert.NotContains(t, string(out), iwtPath, "git must no longer track the integration worktree")

	if entries, err := os.ReadDir(slugDir); err == nil {
		assert.Empty(t, entries, "slug worktree dir must be empty after AllDone GC")
	}
}

// TestDispatcher_HITLOnlyExitPreservesIntegrationWorktree asserts that the HITL
// exit path leaves the per-slug integration worktree on disk so a subsequent
// `pba dispatch` run (after the HITL issue is resolved) can re-enter MergeBack
// against the same worktree without recreating it.
func TestDispatcher_HITLOnlyExitPreservesIntegrationWorktree(t *testing.T) {
	fix := setupDispatch(t)

	// One HITL issue so dispatch exits via ErrHITLOnly. We also pre-merge an
	// AFK issue to force MergeBack to create the iwt before the HITL exit.
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	hitl := mkAFKIssue(2, "decide", "todo")
	hitl.Labels = []string{"HITL"}
	writeIssue(t, fix.plansDir, hitl)
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir)
	installClaudeStub(t, body)

	d := newDispatcher(fix)
	err := timeBoxRun(t, d, "demo", 15*time.Second)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrHITLOnly), "expected ErrHITLOnly, got %v", err)

	parent, err := filepath.EvalSymlinks(filepath.Dir(fix.root))
	require.NoError(t, err)
	iwtPath := filepath.Join(parent, "repo-wt", "demo", "_integration")
	info, statErr := os.Stat(iwtPath)
	require.NoError(t, statErr, "iwt must survive HITL exit")
	assert.True(t, info.IsDir())
}

// TestDispatcher_MergeBackDoesNotMoveParentHEAD asserts the parent repo's HEAD
// is byte-identical before and after a successful dispatch. With MergeBack
// routed through the integration worktree, the parent symbolic ref AND its
// resolved commit must both be unchanged.
func TestDispatcher_MergeBackDoesNotMoveParentHEAD(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"alpha\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/1-alpha.json"
    exit 0
    ;;
esac
exit 1
`, fix.plansDir)
	installClaudeStub(t, body)

	refBefore, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaBefore, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)

	d := newDispatcher(fix)
	require.NoError(t, timeBoxRun(t, d, "demo", 15*time.Second))

	refAfter, err := exec.Command("git", "-C", fix.root, "symbolic-ref", "--short", "HEAD").Output()
	require.NoError(t, err)
	shaAfter, err := exec.Command("git", "-C", fix.root, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(string(refBefore)), strings.TrimSpace(string(refAfter)),
		"parent symbolic ref must not move")
	assert.Equal(t, strings.TrimSpace(string(shaBefore)), strings.TrimSpace(string(shaAfter)),
		"parent HEAD commit must not move")
}

// TestDispatcher_RecoversInReviewWithUnmergedBranch asserts Run reconciles an
// in-review issue whose branch never made it back to integration (prior
// dispatch crashed between `pba complete` and MergeBack). Without recovery,
// ReadyAFK skips the in-review issue and Run errors with "stuck; 0 blocked".
func TestDispatcher_RecoversInReviewWithUnmergedBranch(t *testing.T) {
	fix := setupDispatch(t)

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	// Branch with an unmerged commit, mimicking a sub-agent that committed
	// then exited before MergeBack ran.
	branch := "tester/demo--1-alpha"
	makeMergeableBranch(t, fix.root, integrationBranch, branch, "alpha.txt")

	iss := mkAFKIssue(1, "alpha", "in-review")
	iss.Branch = &branch
	writeIssue(t, fix.plansDir, iss)
	installSkillFile(t, fix.root)

	// Stub fails the test if invoked — recovery must not spawn a sub-agent.
	installClaudeStub(t, "echo 'should not be called' >&2\nexit 99\n")

	require.NoError(t, timeBoxRun(t, d, "demo", 15*time.Second))

	alpha := loadIssueJSON(t, fix.plansDir, 1, "alpha")
	assert.Equal(t, "done", alpha.Status, "in-review issue must be flipped to done after recovery merge")

	logOut, err := exec.Command("git", "-C", fix.root, "log", "--oneline", integrationBranch).CombinedOutput()
	require.NoError(t, err, "git log: %s", string(logOut))
	assert.Contains(t, string(logOut), "merge issue #1", "integration branch must contain merge commit from recovery")
}

// TestDispatcher_RecoveryUnblocksDependents reproduces the dispatch-stuck bug:
// issue 1 in-review with unmerged branch, issue 2 backlog blocked_by [1].
// Without recovery, ReadyAFK skips both (blocker not done) and Run errors with
// "dispatch stuck; 0 blocked". With recovery, issue 1 merges + flips done,
// then ReadyAFK picks up issue 2 and the loop completes normally.
func TestDispatcher_RecoveryUnblocksDependents(t *testing.T) {
	fix := setupDispatch(t)

	d := newDispatcher(fix)
	integrationBranch, err := d.ensureIntegrationBranch(context.Background(), "demo")
	require.NoError(t, err)

	branch1 := "tester/demo--1-first"
	makeMergeableBranch(t, fix.root, integrationBranch, branch1, "first.txt")

	iss1 := mkAFKIssue(1, "first", "in-review")
	iss1.Branch = &branch1
	iss1.Blocking = []int{2}
	writeIssue(t, fix.plansDir, iss1)
	writeIssue(t, fix.plansDir, mkAFKIssue(2, "second", "backlog", 1))
	installSkillFile(t, fix.root)

	body := fmt.Sprintf(`prompt=$(cat)
case "$prompt" in
  *"\"slug\": \"second\""*)
    sed -i.bak 's/"status": "in-progress"/"status": "in-review"/' "%s/demo/issues/2-second.json"
    exit 0
    ;;
esac
echo "unexpected prompt" >&2
exit 1
`, fix.plansDir)
	installClaudeStub(t, body)

	require.NoError(t, timeBoxRun(t, d, "demo", 30*time.Second))

	first := loadIssueJSON(t, fix.plansDir, 1, "first")
	second := loadIssueJSON(t, fix.plansDir, 2, "second")
	assert.Equal(t, "done", first.Status)
	assert.Equal(t, "done", second.Status)
}

// TestDispatcher_RejectsParallelDispatchOnSameSlug asserts a second Run
// against a slug whose dispatch lock is held fails fast (no poll) with an
// error that names both the slug and the absolute lock path. Holding the
// lock from the same process via planrepo.TryFlock exercises the exact same
// flock contention as a second `pba dispatch` process would trigger.
func TestDispatcher_RejectsParallelDispatchOnSameSlug(t *testing.T) {
	fix := setupDispatch(t)
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "todo"))

	lockPath := filepath.Join(fix.plansDir, "demo", ".dispatch.lock")
	release, err := planrepo.TryFlock(lockPath)
	require.NoError(t, err)
	defer release()

	installClaudeStub(t, "echo 'should not be called'\nexit 99\n")

	d := newDispatcher(fix)
	start := time.Now()
	err = d.Run(context.Background(), "demo")
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond, "second Run must fail fast without polling")
	assert.Contains(t, err.Error(), "demo", "error should name the contended slug")
	assert.Contains(t, err.Error(), lockPath, "error should include the absolute lock path")
}

// TestDispatcher_DifferentSlugLockDoesNotBlock asserts that holding one
// slug's dispatch lock does not block dispatch on a different slug — each
// slug's lock is independent so parallel runs across plans are allowed.
func TestDispatcher_DifferentSlugLockDoesNotBlock(t *testing.T) {
	fix := setupDispatch(t)
	// "demo" plan has only done issues; Run should reach snapshot, find
	// AllDone, and return nil — so a successful return proves the lock
	// acquisition phase passed.
	writeIssue(t, fix.plansDir, mkAFKIssue(1, "alpha", "done"))

	otherLock := filepath.Join(fix.plansDir, "other", ".dispatch.lock")
	release, err := planrepo.TryFlock(otherLock)
	require.NoError(t, err)
	defer release()

	installClaudeStub(t, "echo 'should not be called'\nexit 99\n")

	d := newDispatcher(fix)
	require.NoError(t, d.Run(context.Background(), "demo"),
		"a held lock on slug %q must not block dispatch on slug %q", "other", "demo")
}

func TestLinkPlansDir_TolaratesRealSkillsDir(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".claude", "skills"), 0o755))

	// The worktree already has a real, committed .claude/skills directory.
	wtSkills := filepath.Join(wt, ".claude", "skills")
	require.NoError(t, os.MkdirAll(wtSkills, 0o755))
	marker := filepath.Join(wtSkills, "committed.txt")
	require.NoError(t, os.WriteFile(marker, []byte("x"), 0o644))

	var logBuf bytes.Buffer
	require.NoError(t, linkPlansDir(parent, wt, &logBuf))

	info, err := os.Lstat(wtSkills)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	_, err = os.Stat(marker)
	assert.NoError(t, err, "committed file should survive")

	pbInfo, err := os.Lstat(filepath.Join(wt, ".plan-bender"))
	require.NoError(t, err)
	assert.NotZero(t, pbInfo.Mode()&os.ModeSymlink)
}

func TestLinkPlansDir_RejectsRealPlanBenderDir(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	// A real .plan-bender in the worktree would route sub-agent status writes
	// away from the parent's on-disk state — the dispatch loop wouldn't see
	// them. linkPlansDir must reject rather than tolerate.
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".plan-bender"), 0o755))

	var logBuf bytes.Buffer
	err := linkPlansDir(parent, wt, &logBuf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".plan-bender")
	assert.Contains(t, err.Error(), "real path")
}

func TestLinkPlansDir_RefreshesExistingSymlink(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))

	// A stale symlink pointing at the wrong place.
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(wt, ".plan-bender")))

	var logBuf bytes.Buffer
	require.NoError(t, linkPlansDir(parent, wt, &logBuf))

	target, err := os.Readlink(filepath.Join(wt, ".plan-bender"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, ".plan-bender"), target)
}

func TestLinkPlansDir_MissingSourceSkippedSilently(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	var logBuf bytes.Buffer
	require.NoError(t, linkPlansDir(parent, wt, &logBuf))

	_, err := os.Lstat(filepath.Join(wt, ".plan-bender"))
	assert.True(t, os.IsNotExist(err))
	assert.Empty(t, logBuf.String())
}
