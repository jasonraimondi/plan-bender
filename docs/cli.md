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

`write-prd` and `write-issue` read from stdin when no file is given.
