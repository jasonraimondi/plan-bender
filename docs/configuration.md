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
agents:
  claude-code: true
  pi: true
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
  branch_strategy: integration # integration | direct
                               # integration: dispatch creates <user>/<slug> off the
                               #   default branch and merges issue branches there.
                               # direct: dispatch merges issue branches straight into
                               #   the default branch — no integration branch.
  subprocess_timeout: 30m      # Per-subprocess cap on each `claude --print` invocation.
                               # Go duration string ("30m", "2h"). A hung sub-agent
                               # otherwise blocks the dispatch loop forever. Also caps
                               # the lifetime of `before_issue` / `after_issue` hooks.
                               # Validated at config load.

hooks:                         # Shell strings run by `pba dispatch` around its lifecycle
  before_issue: ""             # Runs in the worktree dir before each subprocess.
                               # Non-zero exit blocks the issue and skips the subprocess.
  after_issue: ""              # Runs in the worktree dir after each subprocess (any outcome).
                               # Failures are logged but do not change issue status.
  after_batch: ""              # Runs in the repo root after merge-back. Non-fatal.
                               # Hook lifetime is bounded by pipeline.subprocess_timeout.

issue_schema:
  custom_fields: []            # Add required fields to every issue
  # - name: team
  #   type: enum
  #   required: true
  #   enum_values: [frontend, backend, platform]

review_with_user: false        # Insert a user review step before writing PRDs/issues

report_bugs: false             # Inject a "Bug reports" section into every generated SKILL.md
                               # telling the agent to write pb-error-report-<UTC>.log on failure
                               # and link the user to the GitHub issues page.

update_check: true             # Check for new releases on pb commands

manage_gitignore: false        # Set true to let pb setup add .plan-bender/, .plan-bender.local.yaml,
                               # and agent skill patterns to .gitignore. Leave off if you
                               # manage .gitignore yourself (via template, CI, etc).
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

Use `{{.commands.write_prd}}` in templates instead of hardcoding binary names.

### Template functions

| Function | Signature | Example |
| --- | --- | --- |
| `kebab` | `kebab(s string) string` | `{{"HelloWorld" \| kebab}}` → `hello-world` |
| `join` | `join(sep string, items []string) string` | `{{join ", " .tracks}}` |
| `contains` | `contains(list []string, item string) bool` | `{{if contains .tracks "intent"}}` |
