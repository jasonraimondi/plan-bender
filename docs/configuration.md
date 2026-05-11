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
    "subprocess_timeout": "30m"
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

- `max_points` — cap per issue; forces thin slices.
- `agents` — bool toggles registry defaults; or use object form for per-agent overrides (`project_dir`, `scope`, ...). Supported: `claude-code`, `opencode`, `openclaw`, `pi`.
- `pipeline.skip` — skill names to exclude, e.g. `["bender-interview-me"]`.
- `pipeline.branch_strategy` — `integration` (dispatch creates `<user>/<slug>` off the default branch and merges issue branches there) or `direct` (dispatch merges issue branches straight into the default branch).
- `pipeline.subprocess_timeout` — Go duration string (`"30m"`, `"2h"`). Per-subprocess cap on each `claude --print` invocation; also caps `before_issue` / `after_issue` hooks. Validated at config load.
- `hooks.before_issue` — runs in the worktree dir before each subprocess; non-zero exit blocks the issue and skips the subprocess.
- `hooks.after_issue` — runs in the worktree dir after each subprocess; failures are logged but do not change issue status.
- `hooks.after_batch` — runs in the repo root after merge-back; non-fatal.
- `issue_schema.custom_fields` — add required fields to every issue. Example: `{"name": "team", "type": "enum", "required": true, "enum_values": ["frontend", "backend", "platform"]}`.
- `manage_gitignore` — when `true`, `pb setup` manages `.plan-bender/`, `.plan-bender.local.json`, and agent skill patterns in `.gitignore`. When `false`, `pb doctor` still warns if `.plan-bender.local.json` is not gitignored.
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

Copy a bundled `.skill.tmpl` to `.plan-bender/templates/` and edit. Run `pb setup` to re-render.

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
