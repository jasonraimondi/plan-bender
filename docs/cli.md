# CLI Reference

## `pb` — Human CLI

| Command | What it does |
| --- | --- |
| `pb setup` | Write defaults, generate skills, symlink install (idempotent — re-run after config changes) |
| `pb setup --linear` | Configure Linear integration |
| `pb setup --yes` (`-y`) | Non-interactive mode |
| `pb sync linear push <slug>` | Push local issues to Linear |
| `pb sync linear pull <slug>` | Pull Linear state into local JSON |
| `pb migrate` | One-shot conversion of legacy `.yaml` plan/config files to `.json` (idempotent; supports `--dry-run`) |
| `pb doctor` | Verify installation health |
| `pb self-update` | Update to latest release |
| `pb next <slug>` | Show recommended next issue (formatted text) |
| `pb merge <slug>` | Merge completed (in-review, deps-done) issues into the integration branch in dependency order |
| `pb complete <slug> <id>` | Mark an issue in-review (ready for review); prints the completion marker |
| `pb worktree create <slug> <id>` | Create a git branch and worktree for one issue |
| `pb worktree gc <slug>` | Remove plan-bender worktrees and merged branches for a plan, including the per-slug integration worktree (preserves unmerged) |
| `pb status <slug>` | Per-issue state for a plan: status counts, labels, blocked notes, branch/PR |
| `pb retry <slug> <id>` | Reset a blocked or needs-input issue to `todo` (appends a structured transition note) |
| `pb park <slug> <id>` | Park an in-progress issue as `needs-input` (appends a structured transition note) |
| `pb completion <shell>` | Shell completion — bash, zsh, fish (hidden from `--help`) |
| `pb docs` | Open GitHub repo in browser |
| `pb docs --print` | Print repo URL without opening |
| `pb docs --full` | Print full config reference |

`pb setup` is idempotent. First run writes config, subsequent runs regenerate skills and re-symlink.
Regeneration is a clean rebuild: skills that were renamed, removed, or filtered out (e.g. backend skills with Linear disabled) are deleted from `.plan-bender/skills/`, and the now-dangling symlinks pb installed for them are pruned from each agent's skills dir.
It also backfills a `$schema` reference into existing config files that lack one (for editor validation/autocomplete).
If `.plan-bender.local.json` already exists, no `.plan-bender.json` is created. Set
`manage_gitignore: false` in config to prevent `pb setup` from modifying `.gitignore`.

## `plan-bender-agent` — Agent CLI

All output is JSON. Errors are `{"error": "...", "code": "..."}` with non-zero exit codes.
Codes: `PLAN_NOT_FOUND`, `INVALID_PLAN` (json on disk doesn't parse — includes `file`, `line`, and a `hint`), `VALIDATION_FAILED`, `CONFIG_ERROR`, `INTERNAL`.

| Command | What it does |
| --- | --- |
| `plan-bender-agent context` | Summary of all plans |
| `plan-bender-agent context <slug>` | Full dump — PRD, issues, dependency graph, stats |
| `plan-bender-agent validate <slug>` | Structured validation errors |
| `plan-bender-agent next <slug>` | Recommended next issue (JSON) |
| `plan-bender-agent write-prd <slug> [file]` | Validate + atomically write PRD; creates the plan dir for fresh slugs |
| `plan-bender-agent write-issue <slug> [file]` | Validate + atomically write issue; requires the slug's PRD to exist |
| `plan-bender-agent sync linear push <slug>` | Push local issues to Linear |
| `plan-bender-agent sync linear pull <slug>` | Pull remote state to local |
| `plan-bender-agent archive <slug>` | Move completed plan to `.archive/` |
| `plan-bender-agent merge <slug>` | JSON `{merged: [...], conflicted: [...]}`; merges in-review/deps-done issues into the integration branch in dependency order |
| `plan-bender-agent complete <slug> <id>` | Mark issue in-review; JSON includes the completion `marker` |
| `plan-bender-agent worktree create <slug> <id>` | JSON `{path, branch, status}` — status is the post-claim issue status (`in-progress`) |
| `plan-bender-agent worktree gc <slug>` | JSON `{removed: [...]}`; cleans issue worktrees and the per-slug integration worktree, unmerged branches are preserved and logged to stderr |
| `plan-bender-agent status <slug>` | JSON `{plan, issues}` — per-issue id, status, labels, branch, full notes |
| `plan-bender-agent retry <slug> <id>` | JSON `{status, id, slug, new_status}`; appends a `[date] blocked→todo: retry` (or `needs-input→todo: retry`) note. Refuses any status that is neither blocked nor needs-input. |
| `plan-bender-agent park <slug> <id>` | JSON `{status, id, slug, new_status}`; appends a `[date] in-progress→needs-input: park` note. Refuses non-in-progress status. |

`write-prd` and `write-issue` read from stdin when no file is given (or when the file arg is `-`).

## Merge

`pb merge <slug>` (or `pba merge`) is a standalone, dependency-ordered merge-back — no dispatch loop. It takes the per-slug `.dispatch.lock` at `{plans_dir}/{slug}/.dispatch.lock` (POSIX flock, non-blocking; fails fast with the holding pid if another merge holds it), then merges every issue that is `in-review` with a branch set **and** whose `blocked_by` are all `done` (or are themselves being merged in the same call) into the integration branch in dependency order. Merged issues flip to `done`; a conflicting merge is `git merge --abort`'d and the issue is marked `blocked` while siblings still merge. With nothing in-review it is a no-op. Human output is a one-line summary; agent mode emits `{"merged":[...],"conflicted":[...]}`.

The integration branch is `<git-user>/<slug>`. Set `pipeline.branch_strategy` to `integration` (default — fork `<git-user>/<slug>` off the default branch) or `direct` (merge onto the default branch).

### Integration worktree

Merge-back never touches the parent repo's HEAD. On first merge of a run, `merge` lazy-creates a long-lived per-slug worktree at `{worktree_base}/{repoName}-wt/{slug}/_integration` checked out to `<git-user>/<slug>`. Every subsequent merge resets it on entry (`git merge --abort` → `git reset --hard <integration-branch>` → `git clean -fdx`) so a prior run that crashed mid-merge doesn't poison the next one. All merges and status flips run with cwd = the integration worktree path. The parent repo can stay on any branch with uncommitted changes.

The integration worktree persists across runs of the same slug for fast resumption and is GC'd via `pba worktree gc <slug>`. See [ADR-0003](./adr/0003-merge-in-dedicated-integration-worktree.md) for the rationale.

### Completion marker

A worker signals completion by calling `pba complete <slug> <id>`, which flips the issue JSON to `status: in-review` and prints `<pba:complete issue-id="N"/>` — the **completion marker**. The marker is a progress line for logs and out-of-band tooling; it is **not** the completion signal. The merger keys on the status flip: it picks up an issue only once it is `in-review`. In agent mode the command's JSON response carries the same line in its `marker` field.

## Dispatcher skill

Multi-issue implementation is no longer a CLI subcommand — it's the live agent skill **`bender-implement-prd`**, which drives the harness Workflow tool. The skill runs a loop-until-stable over rounds: **scout** the ready issues (deps all `done`/`canceled` and status in `{backlog, todo, in-progress}` — label-agnostic), spawn one **worktree-isolated worker** (`bender-implement-issue`) per ready issue in parallel, then run `pba merge <slug>` as the **merger**. Each worker returns a structured `completed` / `blocked` / `needs-decision` outcome and self-advances its own status via the CLI (`worktree create` → `in-progress`, `complete` → `in-review`, `park` → `needs-input`); the dispatcher never flips status for them. Between round-sets it batches every `needs-input` decision back to the operator in one prompt, writes the answers into the issue JSON, and resumes with `pb retry`. Once the loop reaches **stable** with every issue `done`, it runs the completion mode the operator chose up front — `merge` (merge the integration branch onto the landing branch), `branch` (leave it local), or `pr` (push and open a PR).

## Recovering from a stuck issue

When an issue ends `blocked`, `pb status <slug>` shows the per-issue state with the failure reason in `notes`. After fixing the underlying problem (build break, missing dep, etc.), `pb retry <slug> <id>` flips the issue back to `todo` and appends a `[date] blocked→todo: retry` note (the prior failure note is preserved as audit trail) so the next round re-picks it. Retry also resumes a `needs-input` issue back to `todo`. Retry refuses any status that is neither `blocked` nor `needs-input` — fix `done`/`in-review`/`canceled` issues by hand if you need to.
