# CLI Reference

## `pb` — Human CLI

| Command | What it does |
| --- | --- |
| `pb setup` | Write defaults, generate skills, symlink install |
| `pb setup --linear` | Configure Linear integration |
| `pb setup --yes` | Non-interactive mode |
| `pb generate` | Regenerate skills from current config (alias: `gen`) |
| `pb sync linear <slug> --from local\|linear` | Sync plan with Linear; `local`=push, `linear`=pull |
| `pb sync linear push <slug>` | Push local issues to Linear |
| `pb sync linear pull <slug>` | Pull Linear state into local YAML |
| `pb doctor` | Verify installation health |
| `pb self-update` | Update to latest release |
| `pb next <slug>` | Show recommended next issue (formatted text) |
| `pb dispatch <slug>` | Run the autonomous implementation loop for a plan |
| `pb complete <slug> <id>` | Flip an issue to in-review and emit the dispatch sentinel |
| `pb worktree create <slug> <id>` | Create a git branch and worktree for one issue |
| `pb worktree gc <slug>` | Remove plan-bender worktrees and branches for a plan |
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
| `plan-bender-agent write-prd <slug> [file]` | Validate + atomically write PRD |
| `plan-bender-agent write-issue <slug> [file]` | Validate + atomically write issue |
| `plan-bender-agent sync linear push <slug>` | Push local issues to Linear |
| `plan-bender-agent sync linear pull <slug>` | Pull remote state to local |
| `plan-bender-agent archive <slug>` | Move completed plan to `.archive/` |
| `plan-bender-agent dispatch <slug>` | Autonomous implementation loop (see below) |
| `plan-bender-agent complete <slug> <id>` | Mark issue in-review + emit completion sentinel |
| `plan-bender-agent worktree create <slug> <id>` | JSON `{path, branch}` |
| `plan-bender-agent worktree gc <slug>` | JSON `{removed: [...]}` |

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
   - For each batch issue, run `before_issue` hook → spawn `claude --print` in the worktree → run `after_issue` hook. Per-issue stdout streams as `[issue-N] …` and lands at `.plan-bender/logs/{slug}/{id}.log`.
   - Merge successful branches into the integration branch in dependency order, flipping each merged issue to `done`. Conflicts mark the issue `blocked` and `git merge --abort`.
   - Run `after_batch` hook in the repo root.

Exit codes: `0` (all done), `2` (HITL-only remain), `1` (other failure, e.g. stuck-on-blocked).

### Completion sentinel

A sub-agent signals completion by calling `pba complete <slug> <id>`. The command flips the issue YAML to `status: in-review` and writes `<pba:complete issue-id="N"/>` to stdout. Dispatch treats a successful subprocess as `exit 0 AND status == in-review`. Exit 0 without the status flip is treated as failure (issue marked `blocked`).
