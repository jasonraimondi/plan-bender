# CLI Reference

## `pb` — Human CLI

| Command | What it does |
| --- | --- |
| `pb setup` | Write defaults, generate skills, symlink install (idempotent — re-run after config changes) |
| `pb setup --linear` | Configure Linear integration |
| `pb setup --yes` | Non-interactive mode |
| `pb sync linear push <slug>` | Push local issues to Linear |
| `pb sync linear pull <slug>` | Pull Linear state into local YAML |
| `pb doctor` | Verify installation health |
| `pb self-update` | Update to latest release |
| `pb next <slug>` | Show recommended next issue (formatted text) |
| `pb dispatch <slug>` | Run the autonomous implementation loop for a plan |
| `pb complete <slug> <id>` | Flip an issue to in-review and emit the dispatch sentinel |
| `pb worktree create <slug> <id>` | Create a git branch and worktree for one issue |
| `pb worktree gc <slug>` | Remove plan-bender worktrees and merged branches for a plan (preserves unmerged) |
| `pb status <slug>` | Per-issue state for a plan: status counts, labels, blocked notes, branch/PR |
| `pb retry <slug> <id>` | Reset a blocked issue to `todo` and clear its failure notes |
| `pb completion <shell>` | Shell completion — bash, zsh, fish |
| `pb docs` | Open GitHub repo in browser |
| `pb docs --print` | Print repo URL without opening |
| `pb docs --full` | Print full config reference |

`pb setup` is idempotent. First run writes config, subsequent runs regenerate skills and re-symlink.
If `.plan-bender.local.yaml` already exists, no `.plan-bender.yaml` is created. Set
`manage_gitignore: false` in config to prevent `pb setup` from modifying `.gitignore`.

## `plan-bender-agent` — Agent CLI

All output is JSON. Errors are `{"error": "...", "code": "..."}` with non-zero exit codes.

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
| `plan-bender-agent complete <slug> <id>` | Mark issue in-review + emit completion sentinel |
| `plan-bender-agent worktree create <slug> <id>` | JSON `{path, branch}` |
| `plan-bender-agent worktree gc <slug>` | JSON `{removed: [...]}`; unmerged branches are preserved and logged to stderr |
| `plan-bender-agent status <slug>` | JSON `{plan, issues}` — per-issue id, status, labels, branch, full notes |
| `plan-bender-agent retry <slug> <id>` | JSON `{status, id, slug, new_status, cleared_notes}`; refuses non-blocked status |

`write-prd` and `write-issue` read from stdin when no file is given.

## Dispatch lifecycle

`pba dispatch <slug>` runs the full implementation loop:

1. Determine the integration branch from `pipeline.branch_strategy`:
   - `integration` (default) — `<git-user>/<slug>` created off the repo default branch.
   - `direct` — the repo default branch itself.
2. Loop until done:
   - Reload issues from disk; if every issue is `done` or `canceled`, exit 0.
   - Compute the AFK batch (`plan.ReadyAFK`): unblocked issues with the `AFK` label and a non-terminal status (excludes `done`, `canceled`, `in-review`, `blocked`).
   - If no batch and only HITL issues remain, print a summary and exit 2.
   - For each batch issue, create the worktree → atomically claim the issue (`status: in-progress` + `branch: <name>` written through the canonical struct round-trip) → run `before_issue` hook → spawn `claude --print` in the worktree → run `after_issue` hook. The pre-spawn claim is what keeps the YAML parseable: without it, sub-agents follow the implement-issue skill's "set branch / set status" instructions and a naive Edit produces duplicate `branch:` keys that yaml.v3 then rejects. Per-issue stdout is serialized through a locked writer and streams as `[issue-N] …`; the full transcript lands at `.plan-bender/logs/{slug}/{id}.log`. Each subprocess is capped by `pipeline.subprocess_timeout` (default `30m`); timeouts mark the issue `blocked` with reason `timed out`.
   - Merge successful branches into the integration branch in dependency order, flipping each merged issue to `done`. Conflicts mark the issue `blocked` and `git merge --abort`. Merge-back is skipped entirely when no issue succeeded in the batch.
   - Run `after_batch` hook in the repo root.

Before merging, dispatch captures the parent repo's HEAD and refuses to run if `git diff-index` reports tracked-file changes. HEAD is restored on exit so successful dispatch never silently leaves the user on the integration branch.

Exit codes: `0` (all done), `2` (HITL-only remain), `1` (other failure, e.g. stuck-on-blocked, dirty repo).

### Completion sentinel

A sub-agent signals completion by calling `pba complete <slug> <id>`. The command flips the issue YAML to `status: in-review` and writes `<pba:complete issue-id="N"/>` to stdout. Dispatch treats a successful subprocess as `exit 0 AND status == in-review`. Exit 0 without the status flip is treated as failure (issue marked `blocked`).

## Recovering from a stuck dispatch

When `pba dispatch` exits non-zero, `pb status <slug>` shows the per-issue state with the failure reason in `notes`. After fixing the underlying problem (build break, missing dep, etc.), `pb retry <slug> <id>` flips the issue back to `todo` and clears its notes so the next dispatch will re-pick it. Retry refuses any non-`blocked` status — fix `done`/`in-review`/`canceled` issues by hand if you need to.
