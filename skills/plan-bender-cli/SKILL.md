---
name: plan-bender-cli
description: Reference for the plan-bender CLI (`pb` and `pba` / `plan-bender-agent`). Use when the user asks about plan-bender commands, the standalone merge command, worktree management, completion marker, the dispatcher skill, recovering a stuck issue, or Linear sync. Also triggers on "what does pb / pba do", "how do I run plan-bender", "pb merge", "pb status", "pb retry", or any question about plan-bender's command surface.
---

# plan-bender CLI

Two binaries ship together:

- `pb` (alias `plan-bender`) — human CLI, formatted output
- `pba` (alias `plan-bender-agent`) — agent CLI, JSON output, what skills shell out to

Same subcommand surface for the most part; `pba` returns structured JSON and well-defined error codes.

## Verify install

```bash
pb doctor
pb --version
```

If missing:

```bash
curl -fsSL https://raw.githubusercontent.com/jasonraimondi/plan-bender/main/install.sh | bash
```

## Setup (run inside any git repo)

```bash
pb setup                # idempotent — writes config, generates skills, symlinks
pb setup --linear       # also configure Linear backend
pb setup --yes          # non-interactive
```

Re-run after config changes; it regenerates skills and re-symlinks. Regeneration is a clean rebuild — skills that were renamed, removed, or filtered out (e.g. backend skills with Linear disabled) are deleted and their dangling symlinks pruned. It also backfills a `$schema` reference into existing config files that lack one. If `.plan-bender.local.json` already exists, no `.plan-bender.json` is created. Set `manage_gitignore: false` to keep `pb setup` from touching `.gitignore`.

## Human CLI (`pb`)

| Command | Purpose |
| --- | --- |
| `pb next <slug>` | Recommended next issue (text) |
| `pb status <slug>` | Per-issue state: status counts, labels, blocked notes, branch/PR |
| `pb merge <slug>` | Merge in-review/deps-done issues into the integration branch in dependency order |
| `pb complete <slug> <id>` | Mark issue `in-review` (ready for review); prints the completion marker |
| `pb retry <slug> <id>` | Reset a `blocked` or `needs-input` issue back to `todo` |
| `pb park <slug> <id>` | Park an `in-progress` issue as `needs-input` |
| `pb worktree create <slug> <id>` | Branch + worktree for one issue |
| `pb worktree gc <slug>` | Remove plan-bender worktrees + merged branches incl. the per-slug integration worktree (keeps unmerged) |
| `pb sync linear push <slug>` | Local JSON → Linear |
| `pb sync linear pull <slug>` | Linear → local JSON |
| `pb migrate` | One-shot legacy `.yaml` → `.json` (supports `--dry-run`) |
| `pb self-update` | Update to latest release |
| `pb completion <shell>` | bash, zsh, fish |
| `pb docs` | Open repo in browser |
| `pb docs --print` | Print repo URL |
| `pb docs --full` | Print full config reference inline |

## Agent CLI (`pba`)

JSON-only output. Errors are `{"error": "...", "code": "..."}` with non-zero exit.

**Error codes:** `PLAN_NOT_FOUND`, `INVALID_PLAN` (includes `file`, `line`, `hint` for parse errors), `VALIDATION_FAILED`, `CONFIG_ERROR`, `INTERNAL`.

| Command | Returns |
| --- | --- |
| `pba context` | Summary of all plans |
| `pba context <slug>` | Full dump — PRD, issues, dep graph, stats |
| `pba validate <slug>` | Structured validation errors |
| `pba next <slug>` | Recommended next issue (JSON) |
| `pba write-prd <slug> [file]` | Validate + atomically write PRD; creates plan dir for fresh slugs |
| `pba write-issue <slug> [file]` | Validate + atomically write issue; requires PRD |
| `pba archive <slug>` | Move completed plan to `.archive/` |
| `pba sync linear push\|pull <slug>` | JSON-emitting variant |
| `pba merge <slug>` | `{merged: [...], conflicted: [...]}`; merges in-review/deps-done issues into the integration branch |
| `pba complete <slug> <id>` | Flip to `in-review` + emit `<pba:complete issue-id="N"/>` |
| `pba worktree create <slug> <id>` | `{path, branch, status}` — status is post-claim (`in-progress`) |
| `pba worktree gc <slug>` | `{removed: [...]}`; cleans issue worktrees + per-slug integration worktree, unmerged branches preserved, logged to stderr |
| `pba status <slug>` | `{plan, issues}` per-issue id, status, labels, branch, notes |
| `pba retry <slug> <id>` | `{status, id, slug, new_status}`; refuses a status that is neither `blocked` nor `needs-input` |
| `pba park <slug> <id>` | `{status, id, slug, new_status}`; refuses non-`in-progress` status |

`write-prd` / `write-issue` read from stdin when no file is given (or when the file arg is `-`).

## Merge

`pb merge <slug>` (or `pba merge`) is a standalone, dependency-ordered merge-back — no dispatch loop. It:

- takes the per-slug `.dispatch.lock` at `{plans_dir}/{slug}/.dispatch.lock` (fails fast if another merge holds it)
- merges every issue that is `in-review` with a branch set **and** whose `blocked_by` are all `done` (or are being merged in the same call) into the integration branch in dependency order
- merged → `done`; a conflict is `git merge --abort`'d and the issue → `blocked` while siblings still merge
- nothing in-review → no-op; agent mode emits `{"merged":[...],"conflicted":[...]}`

The integration branch is `<git-user>/<slug>` (set `pipeline.branch_strategy` to `integration` or `direct`). All git work runs in a long-lived per-slug integration worktree (lazy-created on first merge, reset on entry, GC'd via `pba worktree gc <slug>`); the parent repo's HEAD is never touched and the parent can stay dirty. See [ADR-0003](../../docs/adr/0003-merge-in-dedicated-integration-worktree.md).

### Completion marker

A worker signals completion with `pba complete <slug> <id>`. The command:

- Flips the issue JSON to `status: in-review`
- Prints `<pba:complete issue-id="N"/>` — the **completion marker** (also in the JSON `marker` field in agent mode)

The marker is a progress line for logs and out-of-band tooling; it is **not** the completion signal. The merger keys on the status flip: it picks up an issue only once it is `in-review`.

## Dispatcher skill

Multi-issue implementation is the live agent skill **`bender-implement-prd`** (not a CLI subcommand), which drives the harness Workflow tool in a loop-until-stable: **scout** ready issues (deps `done`/`canceled` + status in `{backlog, todo, in-progress}` — label-agnostic) → spawn one **worktree-isolated worker** (`bender-implement-issue`) per ready issue in parallel → run `pba merge <slug>` as the **merger** → re-scout. Each worker returns a `completed` / `blocked` / `needs-decision` outcome and self-advances its own status via the CLI (`worktree create` → `in-progress`, `complete` → `in-review`, `park` → `needs-input`). Between round-sets the dispatcher batches every `needs-input` decision back to the operator, writes the answers into the issue JSON, and resumes with `pb retry`. Once stable with every issue `done`, it runs the operator-chosen completion mode — `merge`, `branch`, or `pr`.

## Recovering from a stuck issue

```bash
pb status <slug>             # per-issue state; failure reason in `notes`
# fix the underlying problem (build break, missing dep, etc.)
pb retry <slug> <id>         # blocked or needs-input → todo, appends `[date] blocked→todo: retry` note
```

`retry` refuses any status that is neither `blocked` nor `needs-input` — fix `done` / `in-review` / `canceled` by hand if needed. The prior failure note is preserved as audit trail.

## Config layering

Three layers, deep-merged (later wins):

| File | Scope |
| --- | --- |
| `~/.config/plan-bender/defaults.json` | Global, shared across projects |
| `.plan-bender.json` | Project, committed |
| `.plan-bender.local.json` | Project, gitignored — secrets here |

`$VAR` / `${VAR}` are expanded at load time.

## Plan layout

```
.plan-bender/plans/<slug>/
  prd.json
  issues/
    1-setup-middleware.json
    2-add-token-refresh.json
```

`track` ∈ `intent | experience | data | rules | resilience`. `points` hard-capped by `max_points` (default 3). `labels`: `AFK` / `HITL` are escalation **hints** that tune how eagerly a worker parks on a human decision — not gates on which issues are workable (readiness is dependency-graph + status only).

## Discovering more

```bash
pb <command> --help          # any subcommand
pb docs --full               # full config reference
pb docs                      # open repo in browser
```

Repo docs: `docs/cli.md`, `docs/configuration.md`, `docs/schema.md`.
