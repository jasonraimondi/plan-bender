# Changelog

All notable changes to plan-bender are documented here. Format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); plan-bender is pre-1.0 so breaking changes ship in patch releases until v1.

## v0.0.71

### Changed

- All bender skills now set `disable-model-invocation: true` in their frontmatter. The agent can no longer auto-trigger them off conversational phrasing; they run only when explicitly invoked (`/bender-orchestrator`, `/bender-write-plan`, etc.) or dispatched by another skill.

## v0.0.70

### Added

- `pb merge <slug>` / `pba merge`: standalone dependency-ordered merge-back. Acquires the per-slug `.dispatch.lock`, selects in-review issues whose `blocked_by` are all done (or merged in the same call), and reuses the integration-branch machinery — all git work stays inside the integration worktree, so the parent HEAD is never touched (ADR-0003).
- `needs-input` workflow status plus a `park` command to move an issue into it; `retry` is generalized to resume parked issues alongside blocked ones.
- ADR-0006 documenting the harness-driven dispatcher model; CONTEXT dispatch glossary, CLI docs, and README updated to match.

### Changed

- **Breaking:** `bender-implement-prd` is rewritten as a live Workflow dispatcher. Instead of delegating to the `pb dispatch` subcommand, the skill drives the harness Workflow tool directly: a loop-until-stable of scout → parallel worktree-isolated workers → merger, with batched needs-input interviews (AskUserQuestion + retry-resume) between rounds. A cross-run `.dispatcher.lock` guards the loop, distinct from merge's per-round `.dispatch.lock`. Completion-mode pre-flight, merge/branch/pr mechanics, and the status report survive with their triggers adapted from dispatch exit codes to loop-reaches-stable + all-done.
- **Breaking:** `bender-implement-issue` is rewritten as a warm in-session worker contract, replacing the INTEGRATION-vs-STANDALONE framing. The worker claims the issue via worktree create, does all work in that worktree, and returns exactly one structured outcome: completed (ran `complete`), blocked (unrecoverable technical failure), or needs-decision (park to needs-input with a decision payload). The worker no longer integrates — merge-back belongs to the dispatcher.
- Readiness is now label-agnostic: `Ready` returns dependency-satisfied non-terminal issues regardless of AFK/HITL labels, and the `requires_human` computation is gone. Human involvement is handled by the dispatcher's escalation path, not the dep graph.

### Removed

- **Breaking:** the `dispatch` command and the cold `claude --print` subprocess engine (subprocess runner, prompt builder, verdict check, HITL summary, bug-report command). Only the merge-back internals and the hook system survive.
- **Breaking:** the `bender-implement-hitl` skill — its interview logic is folded into the dispatcher's batched needs-input round. The generate pipeline's wipe-and-prune already removes previously generated copies and symlinks.
- **Breaking:** dead config keys `max_parallel` and `subprocess_timeout`.

### Fixed

- The `pb setup` success banner no longer advertises the removed `pb dispatch`; the implementation entry point is now listed as `/bender-implement-prd` under the agent section.

## v0.0.62

### Added

- `no_implement` config flag (default `false`). When `true`, `pb setup` / `pb generate` skip generating the implementation skills (`bender-implement-prd`, `bender-implement-hitl`, `bender-implement-issue`) and drop them from the orchestrator menu. Turning the flag on removes any previously generated copies and prunes their installed symlinks on the next run, reusing the existing wipe-and-prune path.

## v0.0.61

### Removed

- **Breaking:** `bender-write-prd` and `bender-prd-to-issues` skills, both superseded by `bender-write-plan` (which drafts the PRD and decomposes issues in one pass) and slated for removal in >=0.0.60. `pb setup` no longer generates either skill and the pipeline menu no longer lists them. The `plan-bender-agent write-prd` CLI command is retained — `bender-write-plan` still uses it to write `prd.json`.

## v0.0.60

### Added

- `bender-implement-issue` gained an integration-mode preamble (§0). A dispatched sub-agent now detects INTEGRATION vs STANDALONE mode up front (cwd under a `-wt/` worktree, a `--`-suffixed branch, or a prompt issue already in-progress on a branch it didn't create). In integration mode it skips the operator prompts and status hand-edits, then just runs `complete` and exits. The mode was previously only implied by scattered skip-markers, so the sub-agent ran the standalone completion flow and stalled dispatch.

### Fixed

- A dispatched sub-agent's `pba complete` now lands in the parent plans store. Dispatch injects `PLAN_BENDER_PLANS_DIR` (the parent's absolute plans dir) into each sub-agent, so completion writes where `Verdict` looks. Previously, with a git-tracked `plans_dir`, the worktree and parent diverged — the worktree showed in-review while the parent stayed in-progress and Verdict blocked the issue (`subprocess exited 0 but issue status is in-progress, expected in-review`).
- `pb generate` / `pb setup` now prune stale skills. Renamed, removed, or filtered-out skills (e.g. backend skills with Linear disabled) are no longer orphaned in `.plan-bender/skills/` with dangling symlinks in each agent's skills dir. Real directories and symlinks pointing elsewhere (a user's own skills, links from other projects in a shared user-scoped dir) are left intact.
- Command-surface polish: `pb --help` is grouped into Plan workflow / Workspace / Project; `-v` means verbose consistently across `pb` and `pba` (`--version` is long-only); arg-count errors now name the positional args; the internal `completion` command is hidden so it no longer collides with `complete`; plan-not-found drops the leaked `reading prd`/ENOENT chain; `pb setup` gains a `-y` short flag; and "open browser" works on Linux/Windows, not just macOS.
- Documentation: corrected the stale dispatch lifecycle in the `plan-bender-cli` skill (merge-back, status flips, and `after_batch` run in the per-slug integration worktree; the parent repo's HEAD is never touched and a dirty parent is allowed). Renamed "completion sentinel" → "completion marker" throughout — the `<pba:complete/>` line is a human-readable log marker, not dispatch's completion signal (the in-review status flip is).

## v0.0.58

### Added

- `linear.enabled: false` in a project's `.plan-bender.json` is now a live kill-switch. Every Linear read/write re-reads the project file's explicit flag and refuses when it's disabled. Previously a project-level `false` could never override a global enable, because the config merge only ever turned Linear on.

### Fixed

- Merge commit subjects are derived from the issue name (falling back to the branch name) instead of `merge issue #N`. The old subject auto-linked to an unrelated GitHub issue, and the plan-local number is meaningless on the remote.

## v0.0.57

### Changed

- Dispatch skills now run a three-mode completion prompt (merge / branch / pr) instead of an unconditional PR-at-end step. `bender-implement-prd` and standalone `bender-implement-issue` capture the landing branch and ask up front how to land the work, then execute the chosen mode only on a successful (exit 0) dispatch. Per-agent rendering mirrors `bender-review-prd` (claude-code → AskUserQuestion, others → conversation). The merge option hides with a one-line reason when the landing tree is dirty, HEAD is detached, or the landing branch is already the would-be integration branch. See [ADR-0005](docs/adr/0005-dispatch-completion-mode-pre-flight-prompt.md).

## v0.0.56

### Added

- New `bender-write-plan` skill that interviews, drafts the PRD, and decomposes it into issues in a single pass (honoring `review_with_user` at both review checkpoints), replacing the forced handoff between the two prior skills.

### Deprecated

- `bender-write-prd` and `bender-prd-to-issues` are deprecated in favor of `bender-write-plan`. Both still work; they are slated for removal in a future release.

## v0.0.55

### Added

- Generated skills can now ship supporting files. Skill templates moved from flat `{name}.skill.tmpl` files to per-skill `{name}/` directories; generation renders every `*.tmpl` file (stripping the suffix) and copies all other files verbatim, preserving subdirectory structure. See [ADR-0004](docs/adr/0004-skill-templates-as-directories.md).
- Skill templates are validated at load time to have a non-empty body and no output-path collisions, with deterministic (sorted) error output.
- `pb setup` and skill generation warn on stale flat overrides (`{name}.skill.tmpl`) and dir-located forks under `.plan-bender/templates/`.

### Fixed

- A stray override directory under `.plan-bender/templates/` whose name doesn't match a bundled skill no longer bricks the loader for every agent. An override dir is accepted only when its name matches a bundled skill or it ships its own `SKILL.md.tmpl`; others are silently skipped.
- Simplified the `bender-interview-me` skill template.

## v0.0.54

### Fixed

- Dispatch now provisions worktree skills per child and surfaces setup failures (#30). When a repo git-tracks any skill, git recreates `.claude/skills` in every fresh worktree, so the old whole-dir linker skipped it and the gitignored `bender-*` skills never arrived — every issue then blocked at `building prompt: reading skill … bender-implement-issue/SKILL.md`. Skills are now symlinked individually per child (committed skills left in place, idempotent on re-entry), and a still-missing required skill hard-errors at worktree setup. When every ready issue fails setup, dispatch returns one environment error naming the shared cause instead of the misleading `stuck: N blocked`.
- Dispatch writes `pb-error-report-<UTC>.log` to the repo root on any non-HITL failure when `report_bugs` is enabled. Orchestration failures happen in Go before any sub-agent runs, so the agent-facing report prompt never fired — the failure that most needed reporting produced no artifact. Stuck and lock-contention errors are excluded so they don't generate spurious reports.
- Status-commit validation is scoped to the touched issue (#31). A single rule-invalid issue anywhere in the plan no longer blocks status recovery (retry / complete / worktree-create) for every other issue. Authoring/sync commits and `agent validate` keep the strict whole-plan validation.

## v0.0.53

### Added

- `pb setup` backfills a `$schema` key into existing `.plan-bender.json` / `.plan-bender.local.json` files that lack one, preserving formatting and unknown keys.

### Removed

- **`bender-write-prd` and `bender-prd-to-issues` skills removed.** Both were deprecated in favor of `bender-write-plan`, which performs the PRD draft and issue decomposition in a single pass. `pb setup` no longer generates either skill; the planning pipeline menu no longer lists them. Migrate to `/bender-write-plan`. The `plan-bender-agent write-prd` CLI command is unaffected — `bender-write-plan` still uses it to write `prd.json`.

### Fixed

- `write-prd` now creates the PRD even when the slug directory already exists without a `prd.json` (e.g. an empty/leftover dir from a prior partial run). Previously `OpenOrCreate` treated any existing slug dir as loadable and failed with `reading prd {slug}/prd.json: no such file or directory`. A directory that holds an `issues/` tree but no `prd.json` is still surfaced as a loud half-built error.
- A plan with a `prd.json` but no `issues/` directory (a freshly written PRD, before decomposition) now loads and validates as a plan with zero issues, instead of failing with `listing issues in {slug}/issues: no such file or directory`.
- `write-prd` / `write-issue` now accept `-` as the file argument to mean "read from stdin", matching the common Unix convention. Previously `-` was treated as a literal filename.

## v0.0.52

### Added

- Published JSON Schema for `.plan-bender.json` at `schema/plan-bender.schema.json`. Editors that honor `$schema` now offer validation and autocomplete for the config.
- `pb setup` writes a `$schema` key into scaffolded `.plan-bender.json` files, pointing at the published schema so new configs get editor validation out of the box.

### Changed

- Starter config no longer enables the `pi` agent by default; freshly scaffolded configs ship with only `claude-code` enabled.

## v0.0.51

### BREAKING

- **`after_batch` hook cwd changed.** Previously the hook ran with cwd set to the parent repo root. It now runs inside the per-slug integration worktree at `{worktree_base}/{repoName}-wt/{slug}/_integration`, with the just-merged commits checked out. Hooks that resolve paths relative to cwd (e.g. `./scripts/post-merge.sh`, `pnpm test`) will now see the integration worktree's tree rather than the parent repo's. If a hook needs the parent repo root, capture it explicitly before dispatch (e.g. `PB_REPO_ROOT=$(git -C "$(git rev-parse --show-toplevel)" rev-parse --show-toplevel)` is no longer a no-op from inside the worktree — pass it through your environment instead). See [ADR-0003](docs/adr/0003-merge-in-dedicated-integration-worktree.md).

### Added

- `worktree_base` top-level config: relocate dispatch worktrees under any directory. Accepts absolute paths, `~`-prefixed paths, and repo-relative paths (`./...`). Default (`""`) preserves the legacy `{repoParent}/{repoName}-wt/...` layout, so existing users see no path change. See [docs/configuration.md](docs/configuration.md).
- Per-slug dispatch lock at `{plans_dir}/{slug}/.dispatch.lock`. A second `pba dispatch` against the same slug fails fast with the holding pid and lock path. Different slugs in the same repo can now run concurrently.

### Changed

- Merge-back runs entirely inside a dedicated per-slug integration worktree. The parent repo's HEAD is no longer captured, modified, or restored by dispatch, and dispatch no longer refuses to run on a dirty parent working tree. `pba worktree gc <slug>` cleans up the integration worktree along with merged issue worktrees; an in-flight HITL-only exit preserves it for resumption. See [ADR-0003](docs/adr/0003-merge-in-dedicated-integration-worktree.md).
