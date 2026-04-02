# plan-bender

> Structured planning pipeline for AI coding agents — from interview to implementation

## Highlights

- **Full pipeline** — Interview, PRD, issues, review, implement, archive
- **YAML-first** — Plans are version-controllable and mergeable
- **Dual CLI** — Human-friendly `pb` + JSON-only `plan-bender-agent` for agents
- **Multi-agent** — Works with Claude Code, OpenCode, OpenClaw, Pi
- **Thin slices** — Configurable point cap forces small, focused issues
- **Linear sync** — Optional two-way sync with Linear
- **Customizable** — Tracks, workflows, templates, custom fields

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/jasonraimondi/plan-bender/main/install.sh | bash
```

Installs `plan-bender` (aliased `pb`) and `plan-bender-agent` (aliased `pba`) to `~/.local/bin`.

Or with Go:

```sh
go install github.com/jasonraimondi/plan-bender/cmd/plan-bender@latest
go install github.com/jasonraimondi/plan-bender/cmd/plan-bender-agent@latest
```

## Usage

```sh
pb setup
```

Then in Claude Code:

```
/bender-orchestrator
```

The orchestrator reads your plan state and suggests the next action. That's it.

### Pipeline

```
Interview → Write PRD → Break into Issues → Review → Implement → Archive
```

Enter at any point. Skip what you don't need.

## Skills

Skills are the primary interface — agent-side slash commands that drive the pipeline.

| Skill | What it does |
| --- | --- |
| `/bender-orchestrator` | Menu — lists plans, suggests next action |
| `/bender-interview-me` | Stress-test an idea before writing anything |
| `/bender-write-prd` | Interview + explore codebase + write `prd.yaml` |
| `/bender-prd-to-issues` | Decompose PRD into thin vertical-slice issues |
| `/bender-write-issue` | Create a single issue |
| `/bender-review-prd` | Principal-engineer review with auto-fix |
| `/bender-implement-prd` | Work all issues in dependency order |
| `/bender-implement-issue` | One issue end-to-end: branch, code, test, PR |

Exclude skills with `pipeline.skip: [bender-interview-me]` in config.

## CLI

### `pb` — Human CLI

| Command | What it does |
| --- | --- |
| `pb setup` | Write defaults, generate skills, symlink install |
| `pb setup --linear` | Configure Linear integration |
| `pb setup --yes` | Non-interactive mode |
| `pb doctor` | Verify installation health |
| `pb self-update` | Update to latest release |
| `pb completion <shell>` | Shell completion — bash, zsh, fish |

`pb setup` is idempotent. First run writes config, subsequent runs regenerate skills and re-symlink.

### `plan-bender-agent` — Agent CLI

All output is JSON. Errors are `{"error": "...", "code": "..."}` with non-zero exit codes.

| Command | What it does |
| --- | --- |
| `plan-bender-agent context` | Summary of all plans |
| `plan-bender-agent context <slug>` | Full dump — PRD, issues, dependency graph, stats |
| `plan-bender-agent validate <slug>` | Structured validation errors |
| `plan-bender-agent write-prd <slug> [file]` | Validate + atomically write PRD |
| `plan-bender-agent write-issue <slug> [file]` | Validate + atomically write issue |
| `plan-bender-agent sync push <slug>` | Push local issues to Linear |
| `plan-bender-agent sync pull <slug>` | Pull remote state to local |
| `plan-bender-agent archive <slug>` | Move completed plan to `.archive/` |

`write-prd` and `write-issue` read from stdin when no file is given.

## Configuration

Three layers, deep-merged — later wins:

| File | Scope |
| --- | --- |
| `~/.config/plan-bender/defaults.yaml` | Global — shared across projects |
| `.plan-bender.yaml` | Project — committed to repo |
| `.plan-bender.local.yaml` | Local — gitignored, secrets go here |

```yaml
agents:
  - claude-code
plans_dir: ./.plan-bender/plans/
max_points: 3                  # Cap per issue — forces thin slices
step_pattern: "Target — behavior"

tracks:
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
  skip: []

issue_schema:
  custom_fields:
    - name: team
      type: enum
      required: true
      enum_values: [frontend, backend, platform]

# Credentials go in .plan-bender.local.yaml
linear:
  enabled: true
  api_key: "lin_api_..."
  team: "TEAM-ID"
  status_map:
    in-progress: "In Progress"
    in-review: "In Review"
```

Tracks and workflow states are fully customizable.

### Supported agents

| Agent | Skill directory | Scope |
| --- | --- | --- |
| `claude-code` | `.claude/skills/` | Project or user |
| `opencode` | `.opencode/skills/` | Project or user |
| `openclaw` | `~/.openclaw/skills/` | User only |
| `pi` | `.pi/skills/` | Project or user |

## Plan structure

```
plans/
  auth-system/
    prd.yaml
    issues/
      1-setup-middleware.yaml
      2-add-token-refresh.yaml
      3-add-role-checks.yaml
  .archive/
```

### PRD

```yaml
name: "Auth System"
slug: auth-system
status: draft                  # draft | active | complete | archived
created: 2025-03-15
updated: 2025-03-15

description: "JWT-based auth with token refresh and role-based access."
why: "API endpoints are unprotected."
outcome: "All API routes require valid auth."

in_scope:
  - "JWT validation middleware"
  - "Token refresh endpoint"
out_of_scope:
  - "Social login providers"
  - "MFA"

use_cases:
  - id: UC-1
    description: "User logs in, receives access + refresh tokens"

decisions:
  - "Short-lived JWTs (15m) + long-lived refresh tokens (7d)"
open_questions:
  - "Support multiple concurrent sessions per user?"
risks:
  - "Token revocation requires a blocklist store — adds Redis dependency"
validation:
  - "All use cases pass integration tests"
  - "Auth middleware adds < 5ms p99 latency"

dev_command: "npm run dev"
base_url: "http://localhost:3000"
```

### Issue

```yaml
id: 1
slug: setup-middleware
name: "Set up authentication middleware"
track: rules
status: backlog
priority: high                 # urgent | high | medium | low
points: 2                     # 1 to max_points
labels: [AFK]                 # AFK = autonomous, HITL = needs human input
assignee: null
blocked_by: []
blocking: [2, 3]
tdd: true                     # Write tests first
headed: false                 # Verify in browser

outcome: "Auth middleware validates JWTs and attaches user context."
scope: "Middleware only — no login UI, no token issuance."

acceptance_criteria:
  - "Valid JWT → user context on request"
  - "Expired JWT → 401"
  - "Missing JWT → 401"

steps:
  - "Auth middleware — reject missing or malformed Authorization header"
  - "Auth middleware — decode JWT, verify signature and expiry"
  - "Auth middleware — attach decoded user context to request object"

use_cases: [UC-1]
```

**Key fields:**

- **`track`** — Classifies the concern. PRD-to-issues checks track coverage.
- **`tdd`** — Agent writes tests first, then makes them pass.
- **`headed`** — Agent verifies visual outcomes in a browser.
- **`AFK` / `HITL`** — Autonomous vs. needs human input.
- **`blocked_by` / `blocking`** — Dependency graph for implementation ordering.
- **`steps`** — How to build it. Acceptance criteria define *what*; steps define *how*.

## Customizing templates

Copy a bundled `.skill.tmpl` to `.plan-bender/templates/` and edit. Run `pb setup` to re-render.

## Tips

- Start with `/bender-orchestrator` — it reads plan state and suggests next actions.
- Run `/bender-review-prd` before decomposing — catches scope gaps early.
- Keep issues small — `max_points` forces thin slices.
- Use `.plan-bender.local.yaml` for secrets and experiments.

## License

MIT
