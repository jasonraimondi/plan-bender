# Configuration

Three layers, deep-merged — later wins:

| File | Scope |
| --- | --- |
| `~/.config/plan-bender/defaults.yaml` | Global — shared across projects, created manually |
| `.plan-bender.yaml` | Project — committed to repo, written by `pb setup` |
| `.plan-bender.local.yaml` | Local — gitignored, secrets go here |

If `.plan-bender.local.yaml` already exists when you run `pb setup`, no project-level
`.plan-bender.yaml` is created — the loader merges whatever layers exist.

## Default config

What `pb setup` writes on first run:

```yaml
plans_dir: ./.plan-bender/plans/
max_points: 3
agents:
  - claude-code
```

## Kitchen sink

All available keys with their default values:

```yaml
plans_dir: ./.plan-bender/plans/
max_points: 3                  # Cap per issue — forces thin slices
agents:
  - claude-code                # claude-code | opencode | openclaw | pi

tracks:                        # Classify issue concerns; PRD-to-issues checks coverage
  - intent
  - experience
  - data
  - rules
  - resilience

workflow_states:
  - backlog
  - todo
  - in-progress
  - blocked
  - in-review
  - qa
  - done
  - canceled

pipeline:
  skip: []                     # Skill names to exclude, e.g. [bender-interview-me]

issue_schema:
  custom_fields: []            # Add required fields to every issue
  # - name: team
  #   type: enum
  #   required: true
  #   enum_values: [frontend, backend, platform]

review_with_user:              # Skills that include a user review step before writing
  - bender-write-prd           # Set to [] to skip review steps entirely
  - bender-write-issue

update_check: true             # Check for new releases on pb commands

manage_gitignore: true         # Let pb setup add .plan-bender/, .plan-bender.local.yaml,
                               # and agent skill patterns to .gitignore. Set to false
                               # if you manage .gitignore yourself (via template, CI, etc).
                               # When off, pb doctor still warns if .plan-bender.local.yaml
                               # is not gitignored.

# Put this in .plan-bender.local.yaml and load from an env file (e.g. direnv)
# Hardcoding credentials is supported but strongly discouraged
linear:
  enabled: false
  api_key: $LINEAR_API_KEY     # $VAR and ${VAR} are expanded at load time
  team: $LINEAR_TEAM_ID
  project_id: ""               # Optional — scope sync to a project
  status_map:                  # Map local workflow_states to Linear state names
    in-progress: "In Progress"
    in-review: "In Review"
```

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
| `review_with_user` | []string | `review_with_user` config |
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
| `sync` | `plan-bender-agent sync` |
| `archive` | `plan-bender-agent archive` |

Use `{{.commands.write_prd}}` in templates instead of hardcoding binary names.

### Template functions

| Function | Signature | Example |
| --- | --- | --- |
| `kebab` | `kebab(s string) string` | `{{"HelloWorld" \| kebab}}` → `hello-world` |
| `join` | `join(sep string, items []string) string` | `{{join ", " .tracks}}` |
| `contains` | `contains(list []string, item string) bool` | `{{if contains .review_with_user "bender-write-prd"}}` |
