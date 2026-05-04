package cli

// validShipPrd is the minimal-but-valid PRD used by complete/retry tests. It
// passes planrepo.Commit's preflight validation (which the prod status owner
// runs on every save), so status writes don't trip on missing PRD fields.
// UC-1 is declared so cross-ref validation accepts test issues that
// reference it in use_cases.
const validShipPrd = `name: Ship
slug: ship
status: active
created: "2026-04-30"
updated: "2026-04-30"
description: ship it
why: testing
outcome: shipped
use_cases:
  - id: UC-1
    description: ship use case
`

// validAuthPrd is the same shape, used by worktree tests for the "auth" plan.
const validAuthPrd = `name: Auth
slug: auth
status: active
created: "2026-04-30"
updated: "2026-04-30"
description: auth flows
why: testing
outcome: authed
use_cases:
  - id: UC-1
    description: auth use case
`

// validDemoPrd is the same shape, used by dispatch CLI tests for the "demo" plan.
const validDemoPrd = `name: Demo
slug: demo
status: active
created: "2026-04-30"
updated: "2026-04-30"
description: demo plan
why: testing
outcome: demoed
use_cases:
  - id: UC-1
    description: demo use case
`
