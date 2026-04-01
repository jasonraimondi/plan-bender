# plan-bender

> Structured planning pipeline for AI coding agents — interview, PRD, issues, review, implement, archive

Two binaries from one module:

- **`plan-bender`** (`pb`) — human CLI for setup and updates
- **`plan-bender-agent`** — agent CLI, JSON-only, for AI coding agents

Skills (`/bender-*`) are the primary interface. The agent CLI is plumbing: validation, atomic writes, context dumps, sync, and structured JSON output.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/jasonraimondi/plan-bender/main/install.sh | bash
```

## Usage

```sh
pb setup
```

Then in Claude Code:

```
/bender-orchestrator
```

That's it. The orchestrator reads your plan state and suggests the next action.

## Pipeline

```
Interview → Write PRD → Break into Issues → Review → Implement → Archive
```

Each stage is an agent skill. Enter at any point. Skip what you don't need.

## Skills

| Skill | Purpose |
|---|---|
| `/bender-orchestrator` | Menu — shows plans, suggests next action |
| `/bender-interview-me` | Stress-test an idea before writing anything |
| `/bender-write-prd` | Interview + explore codebase + write `prd.yaml` |
| `/bender-prd-to-issues` | Decompose PRD into thin vertical-slice issues |
| `/bender-write-issue` | Create a single issue |
| `/bender-review-prd` | Principal-engineer review with auto-fix |
| `/bender-implement-prd` | Work all issues in dependency order |
| `/bender-implement-issue` | One issue end-to-end: branch, code, test, PR |

Exclude skills with `pipeline.skip: [bender-interview-me]` in config.

## Human CLI (`pb`)

| Command | Purpose |
|---|---|
| `pb setup` | Write defaults + generate skills + symlink install |
| `pb setup --linear` | Configure Linear integration (validates credentials) |
| `pb setup --yes` | Non-interactive mode (for CI) |
| `pb doctor` | Verify installation health (exit 1 on failure) |
| `pb self-update [--force]` | Update to latest release |
| `pb completion <shell>` | Shell completion (bash/zsh/fish) |

`pb setup` (aliased as `pb init`) is idempotent — first run writes default config, subsequent runs regenerate and re-symlink skills. Use `--linear` to enable Linear sync; credentials are read from `$LINEAR_API_KEY`/`$LINEAR_TEAM` env vars or prompted interactively.

## Agent CLI (`plan-bender-agent`)

All output is JSON. Errors are `{"error": "...", "code": "..."}` with non-zero exit codes.

| Command | Purpose |
|---|---|
| `plan-bender-agent context` | Summary of all plans |
| `plan-bender-agent context <slug>` | Full dump: PRD, issues, dependency graph, stats |
| `plan-bender-agent validate <slug>` | Structured errors with severity, file, field, message |
| `plan-bender-agent write-prd <slug> [file]` | Validate + atomically write a PRD |
| `plan-bender-agent write-issue <slug> [file]` | Validate + atomically write an issue |
| `plan-bender-agent sync push <slug>` | Push local issues to Linear |
| `plan-bender-agent sync pull <slug>` | Pull remote state to local |
| `plan-bender-agent archive <slug>` | Move completed plan to `.archive/` |

`write-prd` and `write-issue` read from stdin when no file is given.

## Configuration

Three layers, deep-merged (later wins):

| File | Purpose |
|---|---|
| `~/.config/plan-bender/defaults.yaml` | Shared across projects |
| `.plan-bender.yaml` | Committed to repo |
| `.plan-bender.local.yaml` | Gitignored — secrets, personal overrides |

```yaml
agents:                        # which agents to install skills for
  - claude-code                # .claude/skills/ (project or user scope)
  - openclaw                   # ~/.openclaw/skills/ (user scope)
plans_dir: ./.plan-bender/plans/
max_points: 3                  # cap per issue — forces thin slices
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
  skip: []                     # skill names to exclude

issue_schema:
  custom_fields:
    - name: team
      type: enum
      required: true
      enum_values: [frontend, backend, platform]

# Put credentials in .plan-bender.local.yaml
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
|---|---|---|
| `claude-code` | `.claude/skills/` | Project or user |
| `opencode` | `.opencode/skills/` | Project or user |
| `openclaw` | `~/.openclaw/skills/` | User only |
| `pi` | `.pi/skills/` | Project or user |

### Migrating from `install_target`

If your config uses `install_target`, replace it:

```yaml
# Before
install_target: project

# After
agents:
  - claude-code
```

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
why: "API endpoints are unprotected. Any request can access any resource."
outcome: "All API routes require valid auth. Roles control access. Tokens refresh transparently."

in_scope:
  - "JWT validation middleware"
  - "Token refresh endpoint"
  - "Role-based route guards"
out_of_scope:
  - "Social login providers"
  - "MFA"

use_cases:
  - id: UC-1
    description: "User logs in with email and password, receives access + refresh tokens"

decisions:
  - "Short-lived JWTs (15m) + long-lived refresh tokens (7d)"
open_questions:
  - "Do we need to support multiple concurrent sessions per user?"
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
labels: [AFK]                 # AFK = autonomous | HITL = needs human input
assignee: null
blocked_by: []
blocking: [2, 3]
tdd: true                     # write tests first
headed: false                 # verify in browser

outcome: "Auth middleware validates JWTs and attaches user context to requests."
scope: "Middleware only — no login UI, no token issuance."

acceptance_criteria:
  - "Valid JWT → user context on request"
  - "Expired JWT → 401"
  - "Missing JWT → 401"

steps:
  - "Auth middleware — reject requests with missing or malformed Authorization header"
  - "Auth middleware — decode JWT, verify signature and expiry"
  - "Auth middleware — attach decoded user context to request object"

use_cases: [UC-1]
```

#### Key fields

- **`track`** classifies the concern. PRD-to-issues checks track coverage.
- **`tdd`** — agent writes tests first, then makes them pass.
- **`headed`** — agent verifies visual outcomes in a browser.
- **`AFK` / `HITL`** — autonomous vs. needs human input.
- **`blocked_by` / `blocking`** — dependency graph for implementation ordering.
- **`steps`** — ordered build actions. Steps tell the agent *how*; acceptance criteria tell reviewers *what*.

## Customizing templates

Copy a bundled `.skill.tmpl` to `.plan-bender/templates/` and edit. Run `pb setup` to re-render.

## Tips

- Start with `/bender-orchestrator`. It reads plan state and suggests next actions.
- Run `/bender-review-prd` before decomposing. Catches scope gaps early.
- Keep issues small. `max_points` forces thin slices.
- Use `.plan-bender.local.yaml` for secrets and experiments.

## License

MIT
