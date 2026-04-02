# Schema

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

## PRD

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

## Issue

```yaml
id: 1
slug: setup-middleware
name: "Set up authentication middleware"
track: rules
status: backlog
priority: high                 # urgent | high | medium | low
points: 2                      # 1 to max_points
labels: [AFK]                  # AFK = autonomous, HITL = needs human input
assignee: null
blocked_by: []
blocking: [2, 3]
tdd: true                      # Write tests first
headed: false                  # Verify in browser

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
