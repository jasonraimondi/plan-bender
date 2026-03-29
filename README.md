# plan-bender

> Structured planning pipeline for Claude Code — interview, PRD, issues, review, implement, archive

Skills (`/bender-*`) are the primary interface. The `pb` CLI is plumbing: validation, atomic writes, sync, and `--json` output for scripts.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/jasonraimondi/plan-bender/main/install.sh | bash
```

Or with npm:

```sh
npm install -g @jasonraimondi/plan-bender
```

## Usage

```sh
pb init
pb install
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

Each stage is a Claude Code skill. Enter at any point. Skip what you don't need.

## Skills

| Skill | Purpose |
|---|---|
| `/bender-orchestrator` | Menu — shows plans, suggests next action |
| `/bender-interview-me` | Stress-test an idea before writing anything |
| `/bender-write-a-prd` | Interview + explore codebase + write `prd.yaml` |
| `/bender-prd-to-issues` | Decompose PRD into thin vertical-slice issues |
| `/bender-write-an-issue` | Create a single issue |
| `/bender-review-prd` | Principal-engineer review with auto-fix |
| `/bender-implement-prd` | Work all issues in dependency order |
| `/bender-implement-issue` | One issue end-to-end: branch, code, test, PR |

Exclude skills with `pipeline.skip: [bender-interview-me]` in config.

## CLI

| Command | Purpose |
|---|---|
| `pb init` | Interactive setup |
| `pb install` | Generate skills + symlink into Claude Code |
| `pb status [slug]` | Dashboard — all plans or single plan detail |
| `pb validate <slug>` | Schema checks, cross-refs, cycle detection |
| `pb graph <slug>` | Mermaid dependency graph |
| `pb write-prd <slug> [file]` | Validate + atomically write a PRD |
| `pb write-issue <slug> [file]` | Validate + atomically write an issue |
| `pb sync push <slug>` | Push local issues to Linear |
| `pb sync pull <slug>` | Pull remote state to local |
| `pb archive <slug>` | Move completed plan to `.archive/` |
| `pb completion <shell>` | Shell completion (bash/zsh/fish) |
| `pb self-update [--force]` | Update to latest release |

`status`, `validate`, and `graph` accept `--json`.

`write-prd` and `write-issue` read from stdin when no file is given.

## Configuration

Three layers, deep-merged (later wins):

| File | Purpose |
|---|---|
| `~/.config/plan-bender/defaults.yaml` | Shared across projects |
| `.plan-bender.yaml` | Committed to repo |
| `.plan-bender.local.yaml` | Gitignored — secrets, personal overrides |

```yaml
backend: yaml-fs               # yaml-fs | linear
install_target: project        # project (.claude/skills/) | user (~/.claude/skills/)
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

# Put api_key in .plan-bender.local.yaml
linear:
  api_key: "lin_api_..."
  team: "TEAM-ID"
  status_map:
    in-progress: "In Progress"
    in-review: "In Review"
```

Tracks and workflow states are fully customizable.

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
status: draft                  # draft | active | in-review | approved | complete | archived
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
- **`blocked_by` / `blocking`** — dependency graph for `pb graph` and implementation ordering.
- **`steps`** — ordered build actions. Steps tell the agent *how*; acceptance criteria tell reviewers *what*.

## Customizing templates

Copy a bundled `.skill.tmpl` to `.plan-bender/templates/` and edit. Run `pb install` to re-render.

## Tips

- Start with `/bender-orchestrator`. It reads plan state and suggests next actions.
- Run `/bender-review-prd` before decomposing. Catches scope gaps early.
- Keep issues small. `max_points` forces thin slices.
- Run `pb validate` early. Catches schema errors, missing cross-refs, and cycles.
- Use `.plan-bender.local.yaml` for secrets and experiments.

## License

MIT
