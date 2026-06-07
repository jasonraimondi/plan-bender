package dispatch

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/jasonraimondi/plan-bender/internal/schema"
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
// SKILL.md so prompt assembly succeeds, and an env-resident fake claude on PATH.
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

// installSkillFile writes a stub bender-implement-issue/SKILL.md into root so
// linkSkills' required-skill check passes (it reads from
// {worktreePath}/.claude/skills/...).
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

// TestLinkPlansDir_ProvisionsSkillsPerChildIntoRealDir is the regression test
// for the root-cause bug: when the worktree already has a real .claude/skills
// dir (because git tracks at least one skill), the whole dir can't be
// symlinked, so the parent's gitignored bender-* skills must be linked in
// per child — committed skills left untouched.
func TestLinkPlansDir_ProvisionsSkillsPerChildIntoRealDir(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	// Parent has the required skill plus another, both typically gitignored.
	installSkillFile(t, parent)
	prdSkill := filepath.Join(parent, ".claude", "skills", "bender-implement-prd")
	require.NoError(t, os.MkdirAll(prdSkill, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(prdSkill, "SKILL.md"), []byte("x"), 0o644))

	// The worktree has a real, committed .claude/skills dir with one tracked skill.
	wtSkills := filepath.Join(wt, ".claude", "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(wtSkills, "update-release"), 0o755))
	committed := filepath.Join(wtSkills, "update-release", "SKILL.md")
	require.NoError(t, os.WriteFile(committed, []byte("committed"), 0o644))

	require.NoError(t, linkPlansDir(parent, wt))

	info, err := os.Lstat(wtSkills)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	_, err = os.Stat(committed)
	assert.NoError(t, err, "committed skill must survive")

	for _, name := range []string{"bender-implement-issue", "bender-implement-prd"} {
		li, err := os.Lstat(filepath.Join(wtSkills, name))
		require.NoError(t, err, "%s should be provisioned", name)
		assert.NotZero(t, li.Mode()&os.ModeSymlink, "%s should be a symlink", name)
	}
	_, err = os.Stat(filepath.Join(wtSkills, "bender-implement-issue", "SKILL.md"))
	assert.NoError(t, err, "required skill must resolve through the symlink")

	pbInfo, err := os.Lstat(filepath.Join(wt, ".plan-bender"))
	require.NoError(t, err)
	assert.NotZero(t, pbInfo.Mode()&os.ModeSymlink)
}

// TestLinkPlansDir_LinksSkillsWhenWorktreeHasNone covers the gitignored-skills
// case: the worktree lacks .claude/skills entirely, so it is created and every
// parent skill is symlinked in.
func TestLinkPlansDir_LinksSkillsWhenWorktreeHasNone(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	installSkillFile(t, parent)

	require.NoError(t, linkPlansDir(parent, wt))

	link := filepath.Join(wt, ".claude", "skills", "bender-implement-issue")
	li, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, li.Mode()&os.ModeSymlink)
	_, err = os.Stat(filepath.Join(link, "SKILL.md"))
	assert.NoError(t, err, "required skill must resolve through the symlink")
}

// TestLinkPlansDir_ErrorsWhenRequiredSkillMissing surfaces the failure at link
// time instead of deep in prompt assembly: the parent has a skills dir but not
// the one the prompt needs.
func TestLinkPlansDir_ErrorsWhenRequiredSkillMissing(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".claude", "skills", "some-other-skill"), 0o755))

	err := linkPlansDir(parent, wt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), requiredSkill)
}

func TestLinkPlansDir_RejectsRealPlanBenderDir(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(parent, ".plan-bender"), 0o755))
	// A real .plan-bender in the worktree would route sub-agent status writes
	// away from the parent's on-disk state — the merge loop wouldn't see
	// them. linkPlansDir must reject rather than tolerate.
	require.NoError(t, os.MkdirAll(filepath.Join(wt, ".plan-bender"), 0o755))

	err := linkPlansDir(parent, wt)
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

	require.NoError(t, linkPlansDir(parent, wt))

	target, err := os.Readlink(filepath.Join(wt, ".plan-bender"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, ".plan-bender"), target)
}

func TestLinkPlansDir_MissingSourceSkippedSilently(t *testing.T) {
	parent := t.TempDir()
	wt := t.TempDir()

	require.NoError(t, linkPlansDir(parent, wt))

	_, err := os.Lstat(filepath.Join(wt, ".plan-bender"))
	assert.True(t, os.IsNotExist(err))
}
