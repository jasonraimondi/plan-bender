# Skill Supporting Files Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a skill template ship supporting files (e.g. `CONTEXT-FORMAT.md`) alongside `SKILL.md` so they appear in the generated and installed skill.

**Architecture:** A skill template becomes a directory (`embedded/{name}/SKILL.md.tmpl` + supporting files) instead of a flat `{name}.skill.tmpl`. The loader walks each dir into a flat file-bag (`Skill{Name, Files}`); generation renders `.tmpl` files and copies the rest verbatim into `.plan-bender/skills/{agent}/{name}/`. The existing whole-directory symlink carries supporting files to the agent unchanged. Project overrides move to `.plan-bender/templates/{name}/` and merge at the file level. See `docs/superpowers/specs/2026-05-24-skill-supporting-files-design.md`, `docs/adr/0004-skill-templates-as-directories.md`, and `CONTEXT.md`.

**Tech Stack:** Go 1.24, `embed`/`io/fs`, `text/template`, testify, cobra.

**Conventions:** `sed -i ''` is the BSD/macOS form (this repo is on darwin). Run from repo root unless a step says otherwise. After each task: `go build ./...` then `go test ./... -count=1`, then commit.

---

## Task 1: Restructure to per-skill directories (pure refactor — behavior identical)

This task changes the source layout and the `LoadTemplates` signature without adding any new behavior. There are no supporting files in real skills yet, so generation output is byte-identical (still one `SKILL.md` per skill). It is a single atomic commit because the type-signature change breaks all callers at compile time at once.

**Files:**
- Move: `internal/template/embedded/*.skill.tmpl` → `internal/template/embedded/{name}/SKILL.md.tmpl`
- Rewrite: `internal/template/embed.go`
- Modify: `internal/cli/generate.go` (GenerateSkills loop; drop unused `strings` import)
- Test (migrate): `internal/template/templates_test.go`, `internal/template/render_test.go`, `internal/cli/generate_test.go`, `internal/cli/generate_cmd_test.go`

- [ ] **Step 1: Move the 11 templates into per-skill directories**

Run from repo root:

```bash
cd internal/template/embedded
for f in *.skill.tmpl; do
  name="${f%.skill.tmpl}"
  mkdir -p "$name"
  git mv "$f" "$name/SKILL.md.tmpl"
done
cd -
ls internal/template/embedded
```

Expected: 11 directories (`bender-interview-me`, `bender-write-prd`, …), each containing `SKILL.md.tmpl`, no `.skill.tmpl` files left.

- [ ] **Step 2: Rewrite `internal/template/embed.go`**

Replace the entire file with:

```go
package template

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:embedded
var embeddedFS embed.FS

// Skill is one skill template: every file in the template directory keyed by its
// path relative to that directory (forward slashes), including SKILL.md.tmpl.
type Skill struct {
	Name  string
	Files map[string]string
}

// Main returns the skill body template (the SKILL.md.tmpl content).
func (s Skill) Main() string { return s.Files["SKILL.md.tmpl"] }

// LoadTemplates returns skill templates keyed by skill name. Bundled skills from
// embed.FS load first, then per-skill overrides from .plan-bender/templates/{name}/
// merge in at the file level (an override file replaces or adds an individual file).
func LoadTemplates(projectRoot string) (map[string]Skill, error) {
	skills := make(map[string]Skill)

	if err := fs.WalkDir(embeddedFS, "embedded", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "embedded/")
		name, file, ok := strings.Cut(rel, "/")
		if !ok {
			return nil // a stray top-level file under embedded/ is not a skill
		}
		data, err := fs.ReadFile(embeddedFS, p)
		if err != nil {
			return err
		}
		addSkillFile(skills, name, file, string(data))
		return nil
	}); err != nil {
		return nil, err
	}

	if err := mergeOverrides(skills, filepath.Join(projectRoot, ".plan-bender", "templates")); err != nil {
		return nil, err
	}

	return skills, nil
}

func addSkillFile(skills map[string]Skill, name, file, content string) {
	s, ok := skills[name]
	if !ok {
		s = Skill{Name: name, Files: make(map[string]string)}
		skills[name] = s
	}
	s.Files[file] = content
}

// mergeOverrides walks .plan-bender/templates/{name}/ and upserts each file into
// the matching skill at the file level. A file directly under templates/ (no
// {name}/ dir) is ignored here; generate.go warns about that legacy flat layout.
func mergeOverrides(skills map[string]Skill, overrideRoot string) error {
	info, err := os.Stat(overrideRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(overrideRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(overrideRoot, p)
		if err != nil {
			return err
		}
		name, file, ok := strings.Cut(filepath.ToSlash(rel), "/")
		if !ok {
			return nil // flat file directly under templates/ is no longer a skill
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		addSkillFile(skills, name, file, string(data))
		return nil
	})
}
```

- [ ] **Step 3: Update `GenerateSkills` in `internal/cli/generate.go`**

Replace the body of `GenerateSkills` (currently lines 64-100) with:

```go
// GenerateSkills renders skill templates into .plan-bender/skills/{agent}/ for
// each configured agent and returns the number of skills written.
func GenerateSkills(root string, cfg config.Config, out io.Writer) (int, error) {
	skills, err := tmpl.LoadTemplates(root)
	if err != nil {
		return 0, fmt.Errorf("loading templates: %w", err)
	}

	count := 0
	for _, agent := range cfg.Agents {
		ctx := tmpl.BuildContext(cfg, agent)
		for name, skill := range skills {
			if tmpl.SkillRequiresBackend(name) && !cfg.Linear.Enabled {
				continue
			}
			outDir := filepath.Join(root, ".plan-bender", "skills", agent.Name, name)
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return 0, fmt.Errorf("creating dir %s: %w", outDir, err)
			}
			rendered, err := tmpl.Render(name, skill.Main(), ctx)
			if err != nil {
				return 0, fmt.Errorf("rendering %s: %w", name, err)
			}
			outPath := filepath.Join(outDir, "SKILL.md")
			if err := os.WriteFile(outPath, []byte(rendered), 0o644); err != nil {
				return 0, fmt.Errorf("writing %s: %w", outPath, err)
			}
			count++
		}
	}

	fmt.Fprintf(out, "%d skills generated\n", count)
	return count, nil
}
```

Then remove the now-unused `"strings"` line from the import block at the top of the file (it was only used by the deleted `strings.TrimSuffix`). It is re-added in Task 3.

- [ ] **Step 4: Migrate `internal/template/templates_test.go` keys**

Run the bulk rewrite for index reads (`tmpls["X.skill.tmpl"]` → `tmpls["X"].Main()`):

```bash
sed -i '' -E 's/tmpls\["(bender-[a-z-]+)\.skill\.tmpl"\]/tmpls["\1"].Main()/g' internal/template/templates_test.go
```

Then apply these four edits by hand:

1. The `expected` slice in `TestAllTemplatesLoad` — strip `.skill.tmpl` from each entry so it reads:

```go
	expected := []string{
		"bender-orchestrator",
		"bender-write-prd",
		"bender-write-issue",
		"bender-prd-to-issues",
		"bender-review-prd",
		"bender-implement-prd",
		"bender-implement-hitl",
		"bender-implement-issue",
		"bender-interview-me",
		"bender-sync-linear",
		"bender-retrospective",
	}
```

2. The loop in `TestAllTemplatesRender`:

```go
	for name, skill := range tmpls {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err, "template %s failed to render", name)
			assert.NotEmpty(t, out)
		})
	}
```

3. The loop in `TestAllTemplates_ConditionalBugReportSection`:

```go
	for name, skill := range tmpls {
		t.Run(name+"/off", func(t *testing.T) {
			ctx := fixtureContext()
			ctx["report_bugs"] = false
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err)
			assert.NotContains(t, out, marker)
		})
		t.Run(name+"/on", func(t *testing.T) {
			ctx := fixtureContext()
			ctx["report_bugs"] = true
			out, err := Render(name, skill.Main(), ctx)
			require.NoError(t, err)
			assert.Contains(t, out, marker)
			assert.Contains(t, out, "https://github.com/jasonraimondi/plan-bender/issues")
		})
	}
```

4. In `TestSyncCommands_RenderWithLinearTool`, strip the suffix from the `cases` keys and add `.Main()` to the `tmpls[name]` read and the orchestrator call:

```go
	cases := map[string]string{
		"bender-prd-to-issues": "plan-bender-agent sync linear push",
		"bender-write-issue":   "plan-bender-agent sync linear push",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := Render(name, tmpls[name].Main(), ctx)
			require.NoError(t, err)
			assert.Contains(t, out, want)
			assert.NotContains(t, out, "{{.commands.sync}}")
		})
	}

	out, err := Render("bender-orchestrator", tmpls["bender-orchestrator"].Main(), ctx)
```

- [ ] **Step 5: Migrate `internal/template/render_test.go`**

Change the key in `TestLoadTemplates_Embedded` (line 72):

```go
	assert.Contains(t, tmpls, "bender-orchestrator")
```

Replace `TestLoadTemplates_LocalOverride` (a new skill via override is now a directory):

```go
func TestLoadTemplates_LocalOverride(t *testing.T) {
	dir := t.TempDir()
	overrideDir := filepath.Join(dir, ".plan-bender", "templates", "custom")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "SKILL.md.tmpl"),
		[]byte("custom content"),
		0o644,
	))

	tmpls, err := LoadTemplates(dir)
	require.NoError(t, err)
	assert.Equal(t, "custom content", tmpls["custom"].Main())
}
```

- [ ] **Step 6: Migrate the override test in `internal/cli/generate_test.go`**

Replace `TestGenerateSkills_UsesLocalOverride` (write the override into the new `{name}/SKILL.md.tmpl` location):

```go
func TestGenerateSkills_UsesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	overrideDir := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "SKILL.md.tmpl"),
		[]byte("Custom content for {{.plans_dir}}"),
		0o644,
	))

	cfg, err := config.Load(dir)
	require.NoError(t, err)

	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Custom content for ./.plan-bender/plans/")
}
```

- [ ] **Step 7: Migrate the override test in `internal/cli/generate_cmd_test.go`**

Replace `TestGenerateCmd_RepicksUpTemplateOverride` (override moves to the dir layout):

```go
func TestGenerateCmd_RepicksUpTemplateOverride(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	runGenerateCmd(t)

	skillPath := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "SKILL.md")
	before, err := os.ReadFile(skillPath)
	require.NoError(t, err)

	overrideDir := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "SKILL.md.tmpl"),
		[]byte("OVERRIDE {{.plans_dir}}"),
		0o644,
	))

	runGenerateCmd(t)

	after, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	assert.NotEqual(t, string(before), string(after), "regenerate should pick up the override")
	assert.Contains(t, string(after), "OVERRIDE")
}
```

- [ ] **Step 8: Build and test**

Run:

```bash
go build ./... && go test ./... -count=1
```

Expected: PASS. `internal/template` and `internal/cli` tests green; counts unchanged (10 skills default, 20 for two agents).

- [ ] **Step 9: Commit**

```bash
git add internal/template/embedded internal/template/embed.go internal/cli/generate.go \
  internal/template/templates_test.go internal/template/render_test.go \
  internal/cli/generate_test.go internal/cli/generate_cmd_test.go
git commit -m "refactor: skill templates as per-skill directories"
```

---

## Task 2: Validate skill templates (body marker + output-path collision)

**Files:**
- Modify: `internal/template/embed.go` (add validation call + `validate`; add `fmt`, `sort` imports)
- Test: `internal/template/templates_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/template/templates_test.go` (it already imports `strings`, `testing`, testify; add `"os"` and `"path/filepath"` to the import block):

```go
func TestLoadTemplates_ErrorsWhenOverrideSkillHasNoBody(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".plan-bender", "templates", "brand-new-skill")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTES.md"), []byte("x"), 0o644))

	_, err := LoadTemplates(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "brand-new-skill")
	assert.Contains(t, err.Error(), "SKILL.md.tmpl")
}

func TestLoadTemplates_ErrorsOnOutputCollision(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(base, 0o755))
	// NOTE.md and NOTE.md.tmpl both produce NOTE.md.
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTE.md"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "NOTE.md.tmpl"), []byte("b"), 0o644))

	_, err := LoadTemplates(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both produce")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/template/ -run 'TestLoadTemplates_Errors' -v`
Expected: FAIL (no error returned — validation not implemented yet).

- [ ] **Step 3: Implement `validate`**

In `internal/template/embed.go`, change the import block to add `"fmt"` and `"sort"`:

```go
import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)
```

Insert the validation call into `LoadTemplates` just before its final `return` (after the `mergeOverrides` block):

```go
	if err := validate(skills); err != nil {
		return nil, err
	}

	return skills, nil
```

Add the function at the end of the file:

```go
// validate enforces post-merge invariants: every skill template must contain a
// SKILL.md.tmpl body, and no two source files may produce the same output path
// (e.g. "X.tmpl" and "X", or "SKILL.md.tmpl" and a verbatim "SKILL.md").
func validate(skills map[string]Skill) error {
	var missing []string
	for name, s := range skills {
		if _, ok := s.Files["SKILL.md.tmpl"]; !ok {
			missing = append(missing, name)
		}
		seen := make(map[string]string, len(s.Files))
		for file := range s.Files {
			out := strings.TrimSuffix(file, ".tmpl")
			if prev, ok := seen[out]; ok {
				return fmt.Errorf("skill %q: files %q and %q both produce %q", name, prev, file, out)
			}
			seen[out] = file
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("skill templates missing SKILL.md.tmpl body: %s", strings.Join(missing, ", "))
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/template/ -count=1`
Expected: PASS (new error tests pass; all existing template tests still green — real skills all have a body and no collisions).

- [ ] **Step 5: Commit**

```bash
git add internal/template/embed.go internal/template/templates_test.go
git commit -m "feat: validate skill templates have a body and no output collisions"
```

---

## Task 3: Rework override warnings (location migration + content staleness)

**Files:**
- Modify: `internal/cli/generate.go` (`warnForkedNextTemplates`; re-add `strings` import)
- Test: `internal/cli/generate_test.go`

- [ ] **Step 1: Rewrite the warning tests**

In `internal/cli/generate_test.go`, replace the three forked-template tests so the fork lives at the new `{name}/SKILL.md.tmpl` location, and add a test for the flat-override migration warning. Leave `TestGenerateCmd_NoForkedNextTemplates_NoWarning` unchanged.

```go
func TestGenerateCmd_ForkedImplementPrd_Warns(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	fork := filepath.Join(dir, ".plan-bender", "templates", "bender-implement-prd")
	require.NoError(t, os.MkdirAll(fork, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-orchestrator")
}

func TestGenerateCmd_ForkedOrchestrator_Warns(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	fork := filepath.Join(dir, ".plan-bender", "templates", "bender-orchestrator")
	require.NoError(t, os.MkdirAll(fork, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-orchestrator")
	assert.Contains(t, out, "pba next")
	assert.Contains(t, out, "re-fork")
	assert.NotContains(t, out, "bender-implement-prd")
}

func TestGenerateCmd_BothForks_WarnsBoth(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	for _, name := range []string{"bender-implement-prd", "bender-orchestrator"} {
		fork := filepath.Join(dir, ".plan-bender", "templates", name)
		require.NoError(t, os.MkdirAll(fork, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(fork, "SKILL.md.tmpl"), []byte("---\nname: x\n---\nbody"), 0o644))
	}

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-implement-prd")
	assert.Contains(t, out, "bender-orchestrator")
	assert.Equal(t, 2, strings.Count(out, "warning:"), "expected exactly two warning lines")
}

func TestGenerateCmd_FlatOverride_WarnsMigration(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	overrideDir := filepath.Join(dir, ".plan-bender", "templates")
	require.NoError(t, os.MkdirAll(overrideDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(overrideDir, "bender-interview-me.skill.tmpl"),
		[]byte("stale flat override"),
		0o644,
	))

	cmd := NewGenerateCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	out := stderr.String()
	assert.Contains(t, out, "bender-interview-me.skill.tmpl")
	assert.Contains(t, out, "no longer read")
	assert.Contains(t, out, "bender-interview-me/SKILL.md.tmpl")
	assert.NotContains(t, out, "pba next")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestGenerateCmd_(Forked|BothForks|FlatOverride)' -v`
Expected: FAIL — the old `warnForkedNextTemplates` only detects flat files and emits the `pba next` message, so the dir-located forks produce no warning and the flat file produces the wrong message.

- [ ] **Step 3: Rewrite `warnForkedNextTemplates`**

In `internal/cli/generate.go`, re-add `"strings"` to the import block, then replace `warnForkedNextTemplates` (currently lines 105-123) with:

```go
// warnForkedNextTemplates emits two kinds of stderr warning for stale project
// overrides in .plan-bender/templates/:
//   - a flat {name}.skill.tmpl is no longer read; it must move to
//     {name}/SKILL.md.tmpl, so warn rather than let the fork silently vanish.
//   - a forked copy of a template that upstream now delegates to `pba next`
//     (checked at the new {name}/SKILL.md.tmpl location); re-fork to pick it up.
func warnForkedNextTemplates(root string, stderr io.Writer) {
	overrideDir := filepath.Join(root, ".plan-bender", "templates")
	entries, err := os.ReadDir(overrideDir)
	if err != nil {
		return
	}
	watched := map[string]bool{
		"bender-implement-prd": true,
		"bender-orchestrator":  true,
	}
	for _, e := range entries {
		if !e.IsDir() {
			if strings.HasSuffix(e.Name(), ".skill.tmpl") {
				name := strings.TrimSuffix(e.Name(), ".skill.tmpl")
				fmt.Fprintf(stderr, "warning: flat override %s is no longer read; move it to %s/SKILL.md.tmpl\n", e.Name(), name)
			}
			continue
		}
		if watched[e.Name()] {
			if _, err := os.Stat(filepath.Join(overrideDir, e.Name(), "SKILL.md.tmpl")); err == nil {
				fmt.Fprintf(stderr, "warning: forked template %s/SKILL.md.tmpl found; upstream now uses `pba next` — re-fork to pick up the new behavior\n", e.Name())
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/generate.go internal/cli/generate_test.go
git commit -m "feat: warn on stale flat overrides and dir-located forks"
```

---

## Task 4: Emit supporting files (verbatim + rendered + nested) with stale cleanup

**Files:**
- Modify: `internal/cli/generate.go` (`GenerateSkills`: RemoveAll + new `writeSkill` helper)
- Test: `internal/cli/generate_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/cli/generate_test.go`:

```go
func TestGenerateSkills_EmitsSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Inject supporting files onto an existing skill via override: one verbatim
	// file and one nested .tmpl file.
	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(filepath.Join(base, "refs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CONTEXT-FORMAT.md"), []byte("verbatim {{.plans_dir}}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "refs", "notes.md.tmpl"), []byte("rendered {{.plans_dir}}"), 0o644))

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	out := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me")

	verbatim, err := os.ReadFile(filepath.Join(out, "CONTEXT-FORMAT.md"))
	require.NoError(t, err)
	assert.Equal(t, "verbatim {{.plans_dir}}", string(verbatim), "non-.tmpl file copied byte-for-byte")

	nested, err := os.ReadFile(filepath.Join(out, "refs", "notes.md"))
	require.NoError(t, err)
	assert.Equal(t, "rendered ./.plan-bender/plans/", string(nested), ".tmpl rendered, suffix stripped, subdir preserved")

	_, err = os.Stat(filepath.Join(out, "SKILL.md"))
	require.NoError(t, err, "body still emitted")
}

func TestGenerateSkills_RemovesStaleSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	base := filepath.Join(dir, ".plan-bender", "templates", "bender-interview-me")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "EXTRA.md"), []byte("x"), 0o644))

	cfg, err := config.Load(dir)
	require.NoError(t, err)
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	staleOut := filepath.Join(dir, ".plan-bender", "skills", "claude-code", "bender-interview-me", "EXTRA.md")
	_, err = os.Stat(staleOut)
	require.NoError(t, err, "supporting file present after first generate")

	require.NoError(t, os.Remove(filepath.Join(base, "EXTRA.md")))
	_, err = GenerateSkills(dir, cfg, &strings.Builder{})
	require.NoError(t, err)

	_, err = os.Stat(staleOut)
	assert.True(t, os.IsNotExist(err), "stale supporting file removed on regenerate")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestGenerateSkills_(EmitsSupportingFiles|RemovesStale)' -v`
Expected: FAIL — supporting files are not yet written (only `SKILL.md` is), so the reads error.

- [ ] **Step 3: Add RemoveAll + `writeSkill` to `GenerateSkills`**

In `internal/cli/generate.go`, replace the per-skill inner block of `GenerateSkills` so it clears the dir then writes the whole bag:

```go
		for name, skill := range skills {
			if tmpl.SkillRequiresBackend(name) && !cfg.Linear.Enabled {
				continue
			}
			outDir := filepath.Join(root, ".plan-bender", "skills", agent.Name, name)
			if err := os.RemoveAll(outDir); err != nil {
				return 0, fmt.Errorf("clearing %s: %w", outDir, err)
			}
			if err := writeSkill(name, skill, ctx, outDir); err != nil {
				return 0, err
			}
			count++
		}
```

Add the helper below `GenerateSkills`:

```go
// writeSkill renders or copies every file in a skill template into outDir. Files
// ending in .tmpl are rendered through the template engine and lose the suffix;
// every other file is copied verbatim. Subdirectories are preserved.
func writeSkill(name string, skill tmpl.Skill, ctx any, outDir string) error {
	for file, content := range skill.Files {
		data := content
		outRel := file
		if strings.HasSuffix(file, ".tmpl") {
			outRel = strings.TrimSuffix(file, ".tmpl")
			rendered, err := tmpl.Render(name+"/"+file, content, ctx)
			if err != nil {
				return fmt.Errorf("rendering %s/%s: %w", name, file, err)
			}
			data = rendered
		}
		outPath := filepath.Join(outDir, filepath.FromSlash(outRel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, []byte(data), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
	}
	return nil
}
```

Note: `BuildContext` returns a `map`, so `ctx any` accepts it without importing its concrete type. `strings` is already imported (re-added in Task 3).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -count=1`
Expected: PASS. The existing `TestGenerateSkills_CreatesSkillFiles` / `_MultipleAgents` still pass (real skills have no supporting files, so output is one `SKILL.md` each; counts 10 and 20 unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/generate.go internal/cli/generate_test.go
git commit -m "feat: emit skill supporting files into generated skills"
```

---

## Task 5: Document the directory layout and supporting files

**Files:**
- Modify: `docs/configuration.md` (the "Customizing templates" section, line 109+)

- [ ] **Step 1: Rewrite the "Customizing templates" intro**

In `docs/configuration.md`, replace line 111 ("Copy a bundled `.skill.tmpl` to `.plan-bender/templates/` and edit. Run `pb setup` to re-render.") with:

```md
Override a bundled skill by creating `.plan-bender/templates/{name}/SKILL.md.tmpl` (e.g. `.plan-bender/templates/bender-write-prd/SKILL.md.tmpl`) and editing it. Run `pb setup` to re-render.

A skill template is a directory: the `SKILL.md.tmpl` body plus any **supporting files** beside it. Files ending in `.tmpl` are rendered with the same context and lose the suffix; all other files are copied verbatim. Supporting files (including nested subdirectories) are emitted into the generated skill and reached through the installed-skill symlink, so `SKILL.md` can reference a sibling like `./CONTEXT-FORMAT.md`. Overrides merge per file, so dropping one file into `.plan-bender/templates/{name}/` replaces or adds just that file.

> The pre-directory flat layout (`.plan-bender/templates/{name}.skill.tmpl`) is no longer read; `pb setup` warns if it finds one. Move it to `{name}/SKILL.md.tmpl`.
```

- [ ] **Step 2: Verify the build still compiles (docs-only, but confirm nothing references the old text)**

Run:

```bash
go build ./... && grep -rn "Copy a bundled" docs/configuration.md
```

Expected: build PASS; grep returns nothing (old sentence removed).

- [ ] **Step 3: Commit**

```bash
git add docs/configuration.md
git commit -m "docs: document skill directory layout and supporting files"
```

---

## Self-review notes

- **Spec coverage:** source restructure (T1), flat file-bag + `Main()` (T1), recursive load + file-level merge (T1), body-marker + collision validation (T2), warn-only migration + two warnings (T3), supporting-file emit with render-if-`.tmpl` + nesting + RemoveAll cleanup (T4), docs (T5). Untouched files (`setup.go`, `dispatcher.go`, `doctor.go`) verified to operate on whole skill dirs — no task needed.
- **Out of scope (per spec/ADR):** whole-skill orphan cleanup, auto-migration of flat overrides, supporting-file deletion via override, inline `include`.
- **Type consistency:** `Skill{Name, Files map[string]string}`, `Skill.Main()`, `LoadTemplates → map[string]Skill`, `writeSkill(name, skill, ctx, outDir)`, `validate(skills)` used consistently across tasks.
- **Not touched intentionally:** `docs/research/sandcastle-comparison.md` mentions `.skill.tmpl` as historical prose — left as-is.
```
