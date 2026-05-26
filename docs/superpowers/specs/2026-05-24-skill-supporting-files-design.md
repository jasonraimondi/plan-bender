# Skill templates carry supporting files

## Goal

Let a skill template ship files alongside its body so they appear in the generated and installed skill — e.g. a `CONTEXT-FORMAT.md` that `SKILL.md` references by relative path. Motivating example: porting `grill-with-docs` (a `SKILL.md` plus `CONTEXT-FORMAT.md` / `ADR-FORMAT.md`).

Terminology in [CONTEXT.md](../../../CONTEXT.md); architectural decision in [ADR 0004](../../adr/0004-skill-templates-as-directories.md).

## Design

### Source layout — per-skill directories
A skill template is a directory, not a flat file.
```
internal/template/embedded/
  bender-interview-me/
    SKILL.md.tmpl          # body (was bender-interview-me.skill.tmpl)
    CONTEXT-FORMAT.md      # supporting file, verbatim
    references/notes.md.tmpl   # supporting file, nested + rendered
```
`git mv` all 11 flat templates into `{name}/SKILL.md.tmpl`.

### Data model — flat file-bag
```go
// Skill is one skill template: every file keyed by path relative to the skill dir,
// including SKILL.md.tmpl.
type Skill struct {
    Name  string
    Files map[string]string
}
func (s Skill) Main() string { return s.Files["SKILL.md.tmpl"] }

func LoadTemplates(root string) (map[string]Skill, error) // keyed by skill name
```

### Render rule — uniform, per file
For every file in the bag: ends in `.tmpl` → render through the existing engine with the per-agent context (`BuildContext(cfg, agent)`), strip the suffix; else copy verbatim.
- `SKILL.md.tmpl` → `SKILL.md` (rendered — same rule, not special-cased)
- `CONTEXT-FORMAT.md` → `CONTEXT-FORMAT.md` (verbatim)
- `references/notes.md.tmpl` → `references/notes.md` (rendered, relpath preserved)

### Body marker & validity
- Body is required and always `SKILL.md.tmpl`. A template with no variables passes through unchanged, so the suffix is free.
- Validity is checked **post-merge**: after embedded+override merge, every skill template must contain `SKILL.md.tmpl`. The loader returns an error listing any that don't (loud failure, no silent empty skill).
- Two source files that would write the same output path (e.g. `X.tmpl` and `X`, or `SKILL.md.tmpl` and a verbatim `SKILL.md`) are an error. This subsumes "no verbatim `SKILL.md`" as one general collision check.
- Top-level non-directory entries under `embedded/` are skipped.

### Loading & override merge
- Embed directive: `//go:embed all:embedded` (recursive; `all:` tolerates `_`/`.`-prefixed files).
- Walk `embedded/` recursively → skills keyed by name, each a file-bag keyed by relpath.
- Then walk `.plan-bender/templates/{name}/` recursively and **upsert at file level** into the matching skill's bag. An override may replace `SKILL.md.tmpl`, replace one supporting file, add a new supporting file, or introduce a brand-new skill (new `{name}/SKILL.md.tmpl`).
- No deletion: an override cannot remove an embedded supporting file (no tombstone). YAGNI.

### Generation — destructive per-skill rebuild
`GenerateSkills`, per agent × skill (backend gate `SkillRequiresBackend` unchanged):
1. `os.RemoveAll(.plan-bender/skills/{agent}/{name})` — clears stale supporting files.
2. For each file in the bag: compute output relpath (strip `.tmpl` if rendered), `MkdirAll(filepath.Dir(out))`, write rendered-or-verbatim bytes.
- Count semantics stay "skills generated."
- Safe: `.plan-bender/` is gitignored/generated; installed-skill symlink stores the absolute path so RemoveAll→MkdirAll keeps it valid; generate runs before symlink, sequentially.

### Migration & warnings (in `generate.go`)
Rework `warnForkedNextTemplates` into two stderr warnings:
1. **Location** (new): any flat `*.skill.tmpl` in `.plan-bender/templates/` → "no longer read; move to `{name}/SKILL.md.tmpl`". Warn-only, continue.
2. **Content staleness** (existing): forks of `bender-implement-prd` / `bender-orchestrator` should re-fork for `pba next` — updated to inspect the new `{name}/SKILL.md.tmpl` location.

## Files to change
- `internal/template/embedded/*` — `git mv` 11 files into `{name}/SKILL.md.tmpl`.
- `internal/template/embed.go` — `Skill` type + `Main()`, recursive `LoadTemplates` returning `map[string]Skill`, file-level merge, post-merge validity + collision checks, embed directive.
- `internal/cli/generate.go` — `GenerateSkills` loop (RemoveAll + multi-file write); rework `warnForkedNextTemplates` (two warnings, new location).
- `docs/configuration.md` — "Customizing templates" section: flat copy → `{name}/SKILL.md.tmpl` dir; document supporting files.
- Tests: `internal/template/templates_test.go` (keys `"{name}.skill.tmpl"` → `"{name}"`, content → `.Main()`; loops iterate skills and render `.Main()`); `internal/cli/generate_test.go`.

## Untouched (verified — operate on whole skill dirs)
`internal/cli/setup.go` (`symlinkSkills`), `internal/dispatch/dispatcher.go` (`linkSkills`), `internal/cli/doctor.go` (`skillsCheck`), `render.go`, `funcmap.go`, `context.go`.

## Testing strategy
New/changed tests assert non-trivial outcomes:
- `.tmpl` supporting file renders with context; non-`.tmpl` copied byte-identical.
- Nested relpath preserved in output.
- File-level override merge: override replaces one supporting file, leaves body; override adds a supporting file; override introduces a new skill.
- Post-merge validity error when a skill (incl. override-introduced) lacks `SKILL.md.tmpl`.
- Output-path collision errors.
- `GenerateSkills`: emits SKILL.md + supporting files into the skill dir; a removed supporting file is gone after regenerate (RemoveAll); backend-gated skill (and its supporting files) skipped when Linear disabled.
- Migration warnings fire for a flat override and for a dir-located stale fork.

## Out of scope
- Whole-skill orphan cleanup (pre-existing gap — a backend-gated generated skill lingers when Linear is later disabled).
- Auto-migration of flat overrides.
- Supporting-file deletion via override.
- Inline `include` directive.
