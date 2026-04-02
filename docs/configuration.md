# Configuration

Three layers, deep-merged — later wins:

| File | Scope |
| --- | --- |
| `~/.config/plan-bender/defaults.yaml` | Global — shared across projects, created manually |
| `.plan-bender.yaml` | Project — committed to repo, written by `pb setup` |
| `.plan-bender.local.yaml` | Local — gitignored, secrets go here |

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

update_check: true             # Check for new releases on pb commands

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
