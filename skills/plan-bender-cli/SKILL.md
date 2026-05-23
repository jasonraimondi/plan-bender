---
name: plan-bender-cli
description: Reference for the plan-bender CLI (`pb` and `pba` / `plan-bender-agent`). Use when the user asks about plan-bender commands, dispatch lifecycle, worktree management, completion sentinel, exit codes, recovering a stuck dispatch, or Linear sync. Also triggers on "what does pb / pba do", "how do I run plan-bender", "pb dispatch", "pb status", "pb retry", or any question about plan-bender's command surface.
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

Re-run after config changes; it regenerates skills and re-symlinks. It also backfills a `$schema` reference into existing config files that lack one. If `.plan-bender.local.json` already exists, no `.plan-bender.json` is created. Set `manage_gitignore: false` to keep `pb setup` from touching `.gitignore`.

## Human CLI (`pb`)

| Command | Purpose |
| --- | --- |
| `pb next <slug>` | Recommended next issue (text) |
| `pb status <slug>` | Per-issue state: status counts, labels, blocked notes, branch/PR |
| `pb dispatch <slug>` | Autonomous implementation loop |
| `pb dispatch <slug> --base <ref>` | Override auto-detected default branch (any `git rev-parse` ref) |
| `pb complete <slug> <id>` | Flip issue to `in-review` + emit dispatch sentinel |
| `pb retry <slug> <id>` | Reset a `blocked` issue back to `todo` |
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
| `pba dispatch <slug>` | Autonomous loop (see below) |
| `pba complete <slug> <id>` | Flip to `in-review` + emit `<pba:complete issue-id="N"/>` |
| `pba worktree create <slug> <id>` | `{path, branch, status}` — status is post-claim (`in-progress`) |
| `pba worktree gc <slug>` | `{removed: [...]}`; cleans issue worktrees + per-slug integration worktree, unmerged branches preserved, logged to stderr |
| `pba status <slug>` | `{plan, issues}` per-issue id, status, labels, branch, notes |
| `pba retry <slug> <id>` | `{status, id, slug, new_status}`; refuses non-`blocked` status |

`write-prd` / `write-issue` read from stdin when no file is given (or when the file arg is `-`).

## Dispatch lifecycle

`pba dispatch <slug>` (or `pb dispatch`) runs the full implementation loop:

1. **Resolve integration branch** from `pipeline.branch_strategy`:
   - `integration` (default) — `<git-user>/<slug>` off the repo default branch
   - `direct` — issue branches merge straight onto the default branch
   - `--base <ref>` overrides the auto-detected default. Any `git rev-parse` ref (local branch, `origin/x`, tag, SHA). Invalid refs error before any worktree is created. On a re-run, `--base` is **ignored with a warning** to avoid clobbering merged work.
2. **Loop until done.** Reload issues from disk each iteration; if every issue is `done`/`canceled`, GC the per-slug integration worktree along with any remaining issue worktrees and exit 0. HITL-only or error exits preserve the integration worktree for resumption.
3. **Compute AFK batch** (`plan.ReadyAFK`): unblocked issues with the `AFK` label and a non-terminal status (excludes `done`, `canceled`, `in-review`, `blocked`).
4. **HITL fallback**: if no batch and only HITL issues remain, print a summary and exit 2. Resolve with `/bender-implement-hitl <slug>`.
5. **Per issue in the batch**: create worktree → atomically claim (`status: in-progress` + `branch:` written through a canonical struct round-trip) → `before_issue` hook → spawn `claude --print` in the worktree → `after_issue` hook.
   - Per-subprocess stdout is serialized through a locked writer and streams as `[issue-N] …`
   - Full transcript: `.plan-bender/logs/<slug>/<id>.log`
   - Capped by `pipeline.subprocess_timeout` (default `30m`); timeouts → `blocked`, reason `timed out`
6. **Merge back** successful branches into the integration branch in dependency order, flipping each merged issue to `done`. Conflicts → `blocked` + `git merge --abort`. Merge-back is skipped entirely when no issue succeeded.
7. **`after_batch` hook** runs in the repo root.

Dispatch refuses to run with a dirty working tree (checks `git diff-index` for tracked-file changes). HEAD is captured and restored on exit so a successful run never silently leaves the user on the integration branch.

### Completion sentinel

A sub-agent signals completion with `pba complete <slug> <id>`. The command:

- Flips the issue JSON to `status: in-review`
- Writes `<pba:complete issue-id="N"/>` to stdout

Dispatch treats a subprocess as successful if **exit 0 AND status == in-review**. Exit 0 without the status flip is treated as failure (issue marked `blocked`).

### Exit codes

- `0` — all done
- `2` — only HITL issues remain → run `/bender-implement-hitl <slug>`
- `1` — other failure. Either *stuck-on-blocked* / lock contention (fix the issues and re-run) or a *setup failure* (`dispatch setup failed for every ready issue ...` — an environment problem where no sub-agent ran; the message names the shared cause, e.g. a worktree missing the `bender-implement-issue` skill). With `report_bugs` on, a `1` exit also writes `pb-error-report-<UTC>.log` to the repo root.

## Recovering from a stuck dispatch

```bash
pb status <slug>             # per-issue state; failure reason in `notes`
# fix the underlying problem (build break, missing dep, etc.)
pb retry <slug> <id>         # blocked → todo, appends `[date] blocked→todo: retry` note
pb dispatch <slug>           # resume
```

`retry` refuses any non-`blocked` status — fix `done` / `in-review` / `canceled` by hand if needed. The prior failure note is preserved as audit trail.

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

`track` ∈ `intent | experience | data | rules | resilience`. `points` hard-capped by `max_points` (default 3). `labels`: `AFK` (autonomous) or `HITL` (needs human).

## Discovering more

```bash
pb <command> --help          # any subcommand
pb docs --full               # full config reference
pb docs                      # open repo in browser
```

Repo docs: `docs/cli.md`, `docs/configuration.md`, `docs/schema.md`.
