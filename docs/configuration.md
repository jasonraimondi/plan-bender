# Configuration

Three layers, deep-merged — later wins:

| File | Scope |
| --- | --- |
| `~/.config/plan-bender/defaults.json` | Global — shared across projects, created manually |
| `.plan-bender.json` | Project — committed to repo, written by `pb setup` |
| `.plan-bender.local.json` | Local — gitignored, secrets go here |

If `.plan-bender.local.json` already exists when you run `pb setup`, no project-level
`.plan-bender.json` is created — the loader merges whatever layers exist.

## Default config

What `pb setup` writes on first run:

```json
{
  "plans_dir": "./.plan-bender/plans/",
  "agents": {
    "claude-code": true,
    "pi": true
  }
}
```

## Kitchen sink

All available keys with their default values:

```json
{
  "plans_dir": "./.plan-bender/plans/",
  "worktree_base": "",
  "max_points": 3,
  "agents": {
    "claude-code": true,
    "pi": true
  },
  "tracks": ["intent", "experience", "data", "rules", "resilience"],
  "workflow_states": [
    "backlog", "todo", "in-progress", "blocked",
    "in-review", "qa", "done", "canceled"
  ],
  "pipeline": {
    "skip": [],
    "branch_strategy": "integration",
    "subprocess_timeout": "30m",
    "max_parallel": 3
  },
  "hooks": {
    "before_issue": "",
    "after_issue": "",
    "after_batch": ""
  },
  "issue_schema": {
    "custom_fields": []
  },
  "review_with_user": false,
  "report_bugs": false,
  "interview_with_docs": false,
  "no_implement": false,
  "update_check": true,
  "manage_gitignore": false,
  "linear": {
    "enabled": false,
    "api_key": "$LINEAR_API_KEY",
    "team": "$LINEAR_TEAM_ID",
    "project_id": "",
    "status_map": {
      "in-progress": "In Progress",
      "in-review": "In Review"
    }
  }
}
```

Field notes:

- `worktree_base` — directory under which dispatch creates per-issue and per-slug integration worktrees. Layout is `{worktree_base}/{repoName}-wt/{slug}/{leaf}`; the `-wt` segment namespaces by repo so a single base shared across clones doesn't collide. Accepted shapes:
  - `""` (default) — repo's parent directory, preserving the legacy `{repoParent}/{repoName}-wt/...` layout. Zero migration for existing users.
  - absolute path, e.g. `"/Users/me/code/wt"` — used as-is.
  - `~`-prefixed, e.g. `"~/code/wt"` — expanded against `$HOME` at load time.
  - relative, e.g. `"./.worktrees"` — resolved against the repo root.
- `max_points` — cap per issue; forces thin slices.
- `agents` — bool toggles registry defaults; or use object form for per-agent overrides (`project_dir`, `scope`, ...). Supported: `claude-code`, `opencode`, `openclaw`, `pi`.
- `pipeline.skip` — skill names to exclude, e.g. `["bender-interview-me"]`.
- `pipeline.branch_strategy` — `integration` (dispatch creates `<user>/<slug>` off the default branch and merges issue branches there) or `direct` (dispatch merges issue branches straight into the default branch).
- `pipeline.subprocess_timeout` — Go duration string (`"30m"`, `"2h"`). Per-subprocess cap on each `claude --print` invocation; also caps `before_issue` / `after_issue` hooks. Validated at config load.
- `pipeline.max_parallel` — cap on concurrent `claude` subprocesses inside one `dispatch` batch. Default `3`. Each subprocess is heavy (model API + MCP servers + a git worktree). Must be at least `1`.
- `hooks.before_issue` — runs in the worktree dir before each subprocess; non-zero exit blocks the issue and skips the subprocess.
- `hooks.after_issue` — runs in the worktree dir after each subprocess; failures are logged but do not change issue status.
- `hooks.after_batch` — runs after merge-back with cwd set to the per-slug integration worktree (the merged commits are checked out there); non-fatal. **Breaking change** from earlier versions, which ran it in the parent repo root.
- `issue_schema.custom_fields` — add required fields to every issue. Example: `{"name": "team", "type": "enum", "required": true, "enum_values": ["frontend", "backend", "platform"]}`.
- `manage_gitignore` — when `true`, `pb setup` manages `.plan-bender/`, `.plan-bender.local.json`, and agent skill patterns in `.gitignore`. When `false`, `pb doctor` still warns if `.plan-bender.local.json` is not gitignored.
- `no_implement` — when `true`, `pb setup`/`pb generate` skip generating the implementation skills (`bender-implement-prd`, `bender-implement-hitl`, `bender-implement-issue`) and drop them from the orchestrator menu. Flipping it on removes any previously generated copies on the next run.
- `linear` — put credentials in `.plan-bender.local.json` and load from an env file (e.g. direnv). `$VAR` and `${VAR}` are expanded at load time. `status_map` maps local `workflow_states` to Linear state names.

Tracks and workflow states are fully customizable.

## Supported agents

| Agent | Skill directory | Scope |
| --- | --- | --- |
| `claude-code` | `.claude/skills/` | Project or user |
| `opencode` | `.opencode/skills/` | Project or user |
| `openclaw` | `~/.openclaw/skills/` | User only |
| `pi` | `.pi/skills/` | Project or user |

## Customizing templates

Override a bundled skill by creating `.plan-bender/templates/{name}/SKILL.md.tmpl` (e.g. `.plan-bender/templates/bender-write-plan/SKILL.md.tmpl`) and editing it. Run `pb setup` to re-render.

A skill template is a directory: the `SKILL.md.tmpl` body plus any **supporting files** beside it. Files ending in `.tmpl` are rendered with the same context and lose the suffix; all other files are copied verbatim. Supporting files (including nested subdirectories) are emitted into the generated skill and reached through the installed-skill symlink, so `SKILL.md` can reference a sibling like `./CONTEXT-FORMAT.md`. Overrides merge per file, so dropping one file into `.plan-bender/templates/{name}/` replaces or adds just that file.

> The pre-directory flat layout (`.plan-bender/templates/{name}.skill.tmpl`) is no longer read; `pb setup` warns if it finds one. Move it to `{name}/SKILL.md.tmpl`.

### Template variables

Templates receive a context map built from your config:

| Variable | Type | Source |
| --- | --- | --- |
| `plans_dir` | string | `plans_dir` config |
| `tracks` | []string | `tracks` config |
| `workflow_states` | []string | `workflow_states` config |
| `max_points` | int | `max_points` config |
| `has_backend_sync` | bool | `linear.enabled` config |
| `review_with_user` | bool | `review_with_user` config |
| `report_bugs` | bool | `report_bugs` config |
| `interview_with_docs` | bool | `interview_with_docs` config |
| `agent` | string | Current agent name |
| `commands` | map | CLI command strings (see below) |
| `custom_fields` | []map | `issue_schema.custom_fields` config |
| `track_descriptions` | []map | Built-in descriptions per track |
| `pipeline_phases` | []map | Pipeline phases minus `pipeline.skip` |
| `step_pattern` | string | `"Target — behavior"` |

#### `commands` map

| Key | Value |
| --- | --- |
| `context` | `plan-bender-agent context` |
| `validate` | `plan-bender-agent validate` |
| `write_prd` | `plan-bender-agent write-prd` |
| `write_issue` | `plan-bender-agent write-issue` |
| `sync_push` | `plan-bender-agent sync linear push` |
| `sync_pull` | `plan-bender-agent sync linear pull` |
| `archive` | `plan-bender-agent archive` |
| `next` | `plan-bender-agent next` |
| `status` | `plan-bender-agent status` |
| `dispatch` | `plan-bender-agent dispatch` |
| `complete` | `plan-bender-agent complete` |
| `retry` | `plan-bender-agent retry` |
| `worktree_create` | `plan-bender-agent worktree create` |

Use `{{.commands.write_prd}}` in templates instead of hardcoding binary names.

### Template functions

| Function | Signature | Example |
| --- | --- | --- |
| `kebab` | `kebab(s string) string` | `{{"HelloWorld" \| kebab}}` → `hello-world` |
| `join` | `join(sep string, items []string) string` | `{{join ", " .tracks}}` |
| `contains` | `contains(list []string, item string) bool` | `{{if contains .tracks "intent"}}` |
