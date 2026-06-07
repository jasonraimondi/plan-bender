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
| `pb dispatch <slug>` | Run the autonomous implementation loop for a plan |
| `pb merge <slug>` | Merge completed (in-review, deps-done) issues into the integration branch in dependency order |
| `pb complete <slug> <id>` | Mark an issue in-review (ready for review); prints the completion marker |
| `pb worktree create <slug> <id>` | Create a git branch and worktree for one issue |
| `pb worktree gc <slug>` | Remove plan-bender worktrees and merged branches for a plan, including the per-slug integration worktree (preserves unmerged) |
| `pb status <slug>` | Per-issue state for a plan: status counts, labels, blocked notes, branch/PR |
| `pb retry <slug> <id>` | Reset a blocked issue to `todo` (appends a structured transition note) |
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
| `plan-bender-agent dispatch <slug>` | Autonomous implementation loop (see below) |
| `plan-bender-agent merge <slug>` | JSON `{merged: [...], conflicted: [...]}`; merges in-review/deps-done issues into the integration branch in dependency order |
| `plan-bender-agent complete <slug> <id>` | Mark issue in-review; JSON includes the completion `marker` |
| `plan-bender-agent worktree create <slug> <id>` | JSON `{path, branch, status}` — status is the post-claim issue status (`in-progress`) |
| `plan-bender-agent worktree gc <slug>` | JSON `{removed: [...]}`; cleans issue worktrees and the per-slug integration worktree, unmerged branches are preserved and logged to stderr |
| `plan-bender-agent status <slug>` | JSON `{plan, issues}` — per-issue id, status, labels, branch, full notes |
| `plan-bender-agent retry <slug> <id>` | JSON `{status, id, slug, new_status}`; appends a `[date] blocked→todo: retry` note. Refuses non-blocked status. |

`write-prd` and `write-issue` read from stdin when no file is given (or when the file arg is `-`).

## Dispatch lifecycle

`pba dispatch <slug>` runs the full implementation loop:

1. Determine the integration branch from `pipeline.branch_strategy`:
   - `integration` (default) — `<git-user>/<slug>` created off the repo default branch.
   - `direct` — the repo default branch itself.

   Pass `--base <commit-ish>` to override the auto-detected default branch. Any ref `git rev-parse` accepts is valid (local branch, `origin/x`, tag, SHA); invalid refs error before any worktree is created. Under `integration`, the integration branch is forked off `--base`; under `direct`, issue branches are merged into `--base`. When `<git-user>/<slug>` already exists from a prior run, the flag is honored only at creation — passing `--base` on a re-run emits a warning and reuses the existing branch (re-forking would clobber merged work).
2. Acquire the per-slug dispatch lock at `{plans_dir}/{slug}/.dispatch.lock` (POSIX flock, non-blocking). A second `pba dispatch` on the same slug fails fast with the holding pid and lock path. Different slugs in the same repo can run concurrently.
3. Loop until done:
   - Reload issues from disk; if every issue is `done` or `canceled`, GC the per-slug integration worktree along with any remaining issue worktrees and exit 0. HITL-only or error exits preserve the integration worktree for resumption.
   - Compute the AFK batch (`plan.ReadyAFK`): unblocked issues with the `AFK` label and a non-terminal status (excludes `done`, `canceled`, `in-review`, `blocked`).
   - If no batch and only HITL issues remain, print a summary and exit 2; resolve with `/bender-implement-hitl <slug>`.
   - For each batch issue, create the worktree → atomically claim the issue (`status: in-progress` + `branch: <name>` written through the canonical struct round-trip) → run `before_issue` hook → spawn `claude --print` in the worktree → run `after_issue` hook. The pre-spawn claim is what keeps the issue JSON parseable: without it, sub-agents follow the implement-issue skill's "set branch / set status" instructions and a naive Edit produces duplicate keys that the strict decoder then rejects. Per-issue stdout is serialized through a locked writer and streams as `[issue-N] …`; the full transcript lands at `.plan-bender/logs/{slug}/{id}.log`. Each subprocess is capped by `pipeline.subprocess_timeout` (default `30m`); timeouts mark the issue `blocked` with reason `timed out`.
   - Merge successful branches into the integration branch **inside the per-slug integration worktree** (see below) in dependency order, flipping each merged issue to `done`. Conflicts mark the issue `blocked` and `git merge --abort`. Merge-back is skipped entirely when no issue succeeded in the batch.
   - Run `after_batch` hook with cwd set to the integration worktree (BREAKING — was the parent repo root in prior versions).

### Integration worktree

Merge-back never touches the parent repo's HEAD. On first MergeBack of a run, dispatch lazy-creates a long-lived per-slug worktree at `{worktree_base}/{repoName}-wt/{slug}/_integration` checked out to `<git-user>/<slug>`. Every subsequent MergeBack resets it on entry (`git merge --abort` → `git reset --hard <integration-branch>` → `git clean -fdx`) so a prior run that crashed mid-merge doesn't poison the next one. All merges, status flips, and the `after_batch` hook run with cwd = the integration worktree path. The parent repo can stay on any branch with uncommitted changes; `pb dispatch` no longer captures or restores HEAD and no longer refuses on a dirty parent.

The integration worktree persists across runs of the same slug for fast resumption and is GC'd only on `AllDone` (or unconditionally via `pba worktree gc <slug>`). See [ADR-0003](./adr/0003-merge-in-dedicated-integration-worktree.md) for the rationale.

Exit codes: `0` (all done), `2` (HITL-only remain; run `/bender-implement-hitl`), `1` (other failure). A `1` has two distinct shapes: *stuck-on-blocked* / lock contention (a dependency or operational problem — fix the issues and re-run), and *setup failure* (`dispatch setup failed for every ready issue ...`, an environment problem where no sub-agent ran — re-running won't help until it's fixed; the message names the shared cause). When `report_bugs` is enabled, a *setup failure* (and only that shape) also writes a `pb-error-report-<UTC>.log` to the repo root capturing the command and error — that failure happens in Go before any sub-agent runs, so the agent-facing report_bugs prompt can't cover it. Stuck-on-blocked and lock contention are user-resolvable, not bugs, so they don't write a report.

### Standalone merge

`pb merge <slug>` (or `pba merge`) runs the merge-back step on its own, without the dispatch loop. It takes the same per-slug `.dispatch.lock` (failing fast if a dispatch or another merge holds it), then merges every issue that is `in-review` with a branch set **and** whose `blocked_by` are all done (or are themselves being merged in the same call) into the integration branch in dependency order. Merged issues flip to `done`; a conflicting merge is `git merge --abort`'d and the issue is marked `blocked` while siblings still merge. Like dispatch's merge-back, all git work happens inside the per-slug integration worktree, so the parent repo's HEAD is never touched, and ADR-0003's strategy is preserved. With nothing in-review it is a no-op. Human output is a one-line summary; agent mode emits `{"merged":[...],"conflicted":[...]}`.

### Completion marker

A sub-agent signals completion by calling `pba complete <slug> <id>`, which flips the issue JSON to `status: in-review` and prints `<pba:complete issue-id="N"/>` — the **completion marker**. The marker is a progress line for logs and out-of-band tooling; it is **not** the completion signal. Dispatch keys on the status flip: a successful subprocess is `exit 0 AND status == in-review`, and exit 0 without the flip is treated as failure (issue marked `blocked`). In agent mode the command's JSON response carries the same line in its `marker` field.

## Recovering from a stuck dispatch

When `pba dispatch` exits non-zero, `pb status <slug>` shows the per-issue state with the failure reason in `notes`. After fixing the underlying problem (build break, missing dep, etc.), `pb retry <slug> <id>` flips the issue back to `todo` and appends a `[date] blocked→todo: retry` note (the prior failure note is preserved as audit trail) so the next dispatch will re-pick it. Retry refuses any non-`blocked` status — fix `done`/`in-review`/`canceled` issues by hand if you need to.
