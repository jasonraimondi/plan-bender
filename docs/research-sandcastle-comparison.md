# sandcastle vs plan-bender — comparative research

> Researched 2026-04-30 using six parallel subagents (docs, codebase, workflows × 2 projects).

## TL;DR

**Different layers of the stack, not competitors.** Sandcastle is a TypeScript *runtime* for running coding agents inside sandboxed worktrees with parallel orchestration; it is loud about isolation primitives and silent about workflow. Plan-bender is a Go *methodology* that drives an agent through a planning pipeline (interview → PRD → thin-sliced issues → implementation) via skills written to `.claude/skills/`; it is loud about workflow and silent about isolation. They could compose. Where plan-bender most has to learn from sandcastle is in **the layer below its skills**: sandboxing, agent provider abstraction, completion signaling, and lifecycle hooks — concerns plan-bender currently punts to prose inside SKILL.md.

## At a glance

| | sandcastle | plan-bender |
|---|---|---|
| One-line | "Orchestrate sandboxed coding agents in TypeScript with `sandcastle.run()`" | "Structured planning pipeline for AI coding agents — from interview to implementation" |
| Layer | Runtime / orchestration / sandbox | Planning + skill distribution |
| Language | TypeScript on Node, Effect-native end-to-end | Go 1.24, Cobra |
| Distribution | `npm i @ai-hero/sandcastle` + `bin: sandcastle` | `curl install.sh \| bash`; static binaries `pb` + `pba` |
| Maturity | v0.5.6, ~2.2k stars, MIT | v0.0.31, pre-1.0, two-binary split, goreleaser |
| LoC | ~13k prod / ~21k tests | ~12k total (cmd+internal) |
| Talks to Claude via | Shells out to `claude --print --output-format stream-json -p -` inside a container | Doesn't talk to Claude. Writes skill markdown; the user's Claude reads it |
| User-facing artifact | A hand-edited `.sandcastle/main.mts` script | YAML PRD + issue files in `.plan-bender/plans/{slug}/` |
| Task store | None — delegated to GitHub Issues / Beads (string substitution into prompts) | YAML files behind a `Backend` interface (yamlFS, linear) |
| Workflow states | None for tasks; `<promise>COMPLETE</promise>` stops an iteration | Explicit list: `backlog → todo → in-progress → blocked → in-review → qa → done → canceled` |
| Sandboxing | First-class — `bind-mount`/`isolated`/`none` × Docker/Podman/Vercel/Daytona | None. Runs in the user's checkout |
| Worktrees | First-class TS primitive (`createWorktree()`, `WorktreeManager.ts`) | Prose in `bender-implement-prd.skill.tmpl` instructing the LLM to `git worktree add` |
| Agent dispatch | Pluggable `AgentProvider` (claude-code, codex, opencode, pi) inside Orchestrator | Per-target *output dir* registry (claude-code, openclaw, opencode, pi); dispatch happens inside Claude via the `Task` tool, prompted in markdown |
| Templates | Five runnable `.mts` programs the user edits | Nine `*.skill.tmpl` rendered to SKILL.md and symlinked into agent dirs |
| Hooks lifecycle | `onSandboxReady`/`onIterationStart`/`onIterationEnd`/`onSandboxClose` (host + sandbox) | None — installs no `settings.json` hooks, no MCP, no subagents |
| Voice | Terse, ALL-CAPS sections, XML tag I/O (`<plan>`, `<promise>COMPLETE</promise>`), RALPH persona, RGR mandated | Terse, table-driven runbook, no emoji, "ultrathink" sub-agent prompts, tracer-bullet thin slices, `max_points: 3` cap |

## Where they actually overlap

- Both ship to a hidden dotfolder (`.sandcastle/`, `.plan-bender/`).
- Both believe in autonomous, loop-driven execution with explicit completion signals (sandcastle's `<promise>COMPLETE</promise>`; plan-bender's status flips via `pba write-issue`).
- Both push parallel multi-issue dispatch (sandcastle's `parallel-planner-with-review` template; plan-bender's worktree fan-out in `bender-implement-prd`).
- Both bake in TDD (sandcastle's `implement-prompt.md` mandates RGR; plan-bender adds `tdd: true` and `headed: true` per-issue flags).
- Both ship per-agent prompt branches (sandcastle's backlog-manager `{{LIST_TASKS_COMMAND}}` substitution; plan-bender's `{{if eq .agent "claude-code"}}…{{end}}` template branches).
- Neither ships hooks/MCP/subagents to the user's `.claude/`. Both keep the integration surface narrow.

## Where they sharply differ

1. **Opinion about workflow.** Sandcastle: *"no opinions about workflow, task management, or context sources are imposed"* — verbatim. Plan-bender: a five-track taxonomy (`intent/experience/data/rules/resilience`), a hard 3-point cap, AFK/HITL labels, an 8-state workflow, principal-engineer review pass with auto-fix matrix.
2. **What "isolation" means.** Sandcastle isolates the *runtime* (Docker/Podman/Vercel/Daytona, configurable bind-mount or sync). Plan-bender isolates *changes* (git worktrees off an integration branch) and trusts the local environment.
3. **Where decision-making lives.** Sandcastle's planner *prompts the LLM* to emit a `<plan>` JSON dependency graph. Plan-bender refuses to: dependency ordering is a deterministic Go function (`internal/plan/next.go:46-180` — `Resolve` returns `Result{Issue, Reason, WasBlocked, RequiresHuman, AllDone, BlockedCount, Skipped}`).
4. **Persistence model.** Sandcastle: no task store; GitHub Issues / Beads via prompt substitution. Plan-bender: YAML on disk behind a `Backend` interface, with Linear as a proper adapter (`internal/backend/backend.go:34-40`).
5. **Templates as code vs templates as docs.** Sandcastle's `main.mts` *is* the orchestration program — users edit it; the library is Effect-Layer plumbing. Plan-bender's templates are markdown skills; the orchestration logic is in the skill prompt and in `pba` Go code.
6. **Two-binary split.** Plan-bender's `pb`/`pba` split (human TUI vs agent JSON) has no analog in sandcastle.

## Best ideas plan-bender could steal from sandcastle

These are ranked by leverage. The pattern: plan-bender currently delegates plumbing to *prose inside SKILL.md*; sandcastle treats those same concerns as first-class abstractions with type signatures.

### 1. Pluggable `AgentProvider` interface in Go (highest leverage)

Sandcastle's `AgentProvider` (`src/AgentProvider.ts:111-122`) abstracts agent invocation: `buildPrintCommand`, `parseStreamLine`, `parseSessionUsage`. Concrete impls for claude-code, codex, opencode, pi.

Plan-bender today: agent dispatch is hardcoded into `bender-implement-prd.skill.tmpl` as "spawn one Agent (Task tool) per issue." That works only inside Claude Code; opencode/openclaw/pi users get the same prose without their dispatch primitives. Fix: move parallel dispatch from prose into a Go-side `pba dispatch` subcommand backed by an `AgentProvider` interface. The skill becomes one line: `pba dispatch <slug>`. This also opens the door to running plan-bender headlessly from CI.

### 2. First-class worktree management in Go

`createWorktree()` and `WorktreeManager.ts` in sandcastle handle `git worktree add`, temp-branch generation, cleanup, and Windows path patching as type-checked code with explicit lifecycle. Plan-bender today asks the LLM to run `git worktree add ../<repo>-wt/<id>-<issue-slug>` — a string of bash inside markdown (`bender-implement-prd.skill.tmpl:66-101`). LLMs sometimes pick wrong paths, forget to clean up failed worktrees, or collide on branch names. A `pba worktree create <slug> <issue-id>` that returns the absolute path (and a corresponding `pba worktree gc`) would remove a class of agent footguns. Keep the prose, but make it call the binary.

### 3. Optional sandbox provider for AFK runs

The single largest gap. Plan-bender AFK issues run in the user's checkout with full network and shell access. Sandcastle's discriminated-union `SandboxProvider` (`bind-mount` | `isolated` | `none`) lets users opt into Docker/Podman isolation for unattended runs without changing the rest of the API. Plan-bender could add `pba run-issue --sandbox docker` as an optional flag that wraps the worktree in a container, leaving the default behavior unchanged. This is a natural fit for the `Backend`-style adapter pattern plan-bender already enforces.

### 4. Completion signaling

Sandcastle's hard-stop convention (`<promise>COMPLETE</promise>` in stdout) is detected by the orchestrator regardless of token budget. Plan-bender relies on the agent remembering to call `pba write-issue` to flip status to `in-review`/`done`. A symmetric convention — e.g. a sentinel string the dispatcher watches for, or `pba complete <slug> <issue-id>` as a one-liner the skill ends with — would make completion detectable from outside the agent's context window.

### 5. Lifecycle hooks

Sandcastle's `onSandboxReady` / `onIterationStart` / `onIterationEnd` / `onSandboxClose` (in `SandboxLifecycle.ts`) let users wire `pnpm install`, `bundle exec rspec`, `bun typecheck`, etc. into the loop without editing prompts. Plan-bender has nothing analogous. Adding a `hooks:` block to `.plan-bender.yaml` (with `before_issue:`, `after_issue:`, `before_pr:`) would give projects a clean place to put repo-specific commands instead of asking each issue prompt to remember them.

### 6. Branch strategy as a configurable enum

Sandcastle's `BranchStrategy = "head" | "merge-to-head" | "branch"` lets the user choose how agent commits relate to HEAD. Plan-bender hardcodes the integration-branch + per-issue-branch + combined-PR model. Surfacing this as `pipeline.branch_strategy: integration | direct | per-issue-pr` in config would let solo devs skip the integration branch ceremony.

### 7. Stream-json observation

Sandcastle parses Claude's `--output-format stream-json` line-by-line into typed events for live UI and idle-timeout. Plan-bender has zero observability into spawned Task sub-agents — once dispatched, they're a black box until they finish or hang. A `pba watch <slug>` that tails an iteration log (written by the dispatcher) would be a real ergonomics win for parallel runs.

### 8. Iteration timeout

Sandcastle uses an Effect `Deferred` race to kill an agent that emits no output for N seconds. Plan-bender has no abort mechanism — a stuck Task agent runs until it OOMs or the user notices. Worth adding once dispatch moves out of prose.

### 9. Session capture for resumability

Sandcastle copies the Claude session `*.jsonl` out of the sandbox after each iteration (`SessionStore.transferSession`, rewriting `cwd` fields). Plan-bender has no resume story for failed issues. Lower priority than the items above.

## Best ideas plan-bender already does better than sandcastle

Worth keeping; sandcastle has nothing like these.

1. **The pure-function next-issue resolver** (`internal/plan/next.go`). Sandcastle asks the LLM to derive a dep graph in JSON every run; plan-bender computes it deterministically from YAML. Predictable, table-test-covered, debuggable.
2. **Track taxonomy + 3-point cap.** The `intent/experience/data/rules/resilience` schema with forced thin-slicing is a real PM opinion that survives multiple agents and review passes.
3. **AFK/HITL as first-class labels** with resolver semantics ("if any AFK is ready, drop all HITL from the pool"). Sandcastle has no human-gating story.
4. **Two-binary split (`pb`/`pba`).** Cleaner human-vs-machine surface than sandcastle's single CLI. Errors as `{"error","code"}` JSON across `pba` is a small detail with big agent-side payoff.
5. **`Backend` adapter for Linear** (`internal/backend/backend.go:34-40`). Sandcastle's "shell out to `gh issue list`" with `{{LIST_TASKS_COMMAND}}` substitution is more flexible but less typed/testable.
6. **Static Go binary, no runtime deps.** Sandcastle requires Node + Docker + the `claude` CLI baked into an image. Plan-bender drops in via curl|bash and works.
7. **`generate` warns on forked templates** when upstream moves logic into `pba` commands. No equivalent in sandcastle's `init`.

## Could they compose?

Yes, in principle. Plan-bender writes the YAML plan; the dispatcher in `bender-implement-prd` could shell out to sandcastle to run each issue inside a container instead of spawning a Task agent. Practical blockers: language mismatch (Go orchestrator + npm-distributed Node runtime + `claude` baked into a Docker image is a heavy ask for a Go-CLI user), and sandcastle assumes the user wrote a `main.mts` to drive its loop. The lighter integration: plan-bender borrows sandcastle's *patterns* (AgentProvider, BranchStrategy, lifecycle hooks, completion sentinel) without depending on the package.

## Bottom line

The single biggest improvement plan-bender could make, based on sandcastle's example, is to **stop encoding orchestration mechanics in markdown and start encoding them in `pba` subcommands behind typed Go interfaces.** Worktree creation, agent dispatch, completion detection, and lifecycle hooks are all currently prose in `bender-implement-prd.skill.tmpl`. Sandcastle treats each of those as a first-class abstraction with adapters and tests. The skill prompts shrink to "run `pba dispatch <slug>` and follow its output," which is more reliable across agents, observable from outside the LLM context, and unit-testable. The opinionated planning layer — the part that's actually plan-bender's product — stays exactly as it is.
