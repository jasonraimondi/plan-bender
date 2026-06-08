# plan-bender

Domain language for plan-bender. This glossary captures terms specific to the project's contexts as they are resolved. It is a glossary, not a spec — no implementation details.

## Language

### Dispatch

**Dispatcher**:
The live **agent** that drives a PRD's issues to done — the `bender-implement-prd` skill running a loop-until-stable of scout → parallel warm workers → merger via the harness **Workflow** tool. It is **not** the Go binary: Go is the state store + git mechanics (status ownership, planrepo, and the dependency-ordered merge-back behind `pba merge`). The dispatcher never flips issue status itself — workers self-advance through the CLI.
_Avoid_: dispatch binary, dispatch process, orchestrator (the Go engine is gone)

**Integration branch**:
The branch the dispatcher builds work onto, named `<user>/<slug>`. Forked from the `base` (default branch unless `--base` is passed). Per-issue worktrees merge into it; the merger never touches the parent repo's HEAD.
_Avoid_: feature branch, work branch, dispatch branch

**Landing branch**:
The branch the parent repo's HEAD is on when the operator invokes a dispatch skill. Distinct from `base` (where the integration branch forks from) and from the integration branch itself. Used as the merge destination when the operator picks the `merge` completion mode.
_Avoid_: target branch, starting branch, current branch (ambiguous), feature branch

**Completion signal**:
What marks a worker as succeeded: the issue's status flipped to `in-review` (via `pba complete`). The **merger** (`pba merge`) reads this status to integrate the issue, in dependency order. The signal is the status, never the printed line.
_Avoid_: sentinel, completion sentinel

**Completion marker**:
The `<pba:complete issue-id="N"/>` line `pba complete` prints to stdout (and carries in its JSON `marker` field). A progress marker for logs and out-of-band tooling — decorative, **not** the completion signal.
_Avoid_: sentinel, completion sentinel

**needs-input**:
A first-class status for a parked human decision. A worker that hits a genuine product/policy/UX ambiguity commits its WIP and `park`s the issue to `needs-input`; it **persists across runs** and is **excluded from readiness**. The dispatcher batch-interviews all parked decisions between rounds, writes the answers back into the issue JSON, then `retry`-resumes the issue (`needs-input` → `todo`). Distinct from `blocked`, which is an unrecoverable **technical** failure, not a human decision.
_Avoid_: paused, waiting, on-hold

**Escalation hint (AFK/HITL labels)**:
The `AFK`/`HITL` labels now only tune a worker's escalation threshold — HITL-labeled issues escalate eagerly on any ambiguity; AFK/unlabeled issues proceed on a stated low-risk assumption and escalate only when truly stuck or the decision is irreversible. They are **not** selection gates: readiness is purely the dependency graph + status.
_Avoid_: routing label, selection gate

### Skill pipeline

**Skill template**:
The authored source for a skill — a directory holding the skill body (`SKILL.md.tmpl`) plus any supporting files; bundled into the binary and optionally overridden per project.
_Avoid_: source skill, template (ambiguous on its own)

**Supporting file**:
Any file in a skill template other than `SKILL.md.tmpl`, e.g. `CONTEXT-FORMAT.md`. Rendered through the template engine if it ends in `.tmpl`, copied verbatim otherwise.
_Avoid_: resource, asset, attachment

**Generated skill**:
The rendered output of a skill template for one configured agent, written to `.plan-bender/skills/{agent}/{name}/`.
_Avoid_: built skill

**Installed skill**:
The directory symlink in an agent's skills directory (e.g. `.claude/skills/{name}/`) that points at a generated skill; the generated skill's supporting files travel through it.
_Avoid_: linked skill

## Relationships

- A **skill template** renders to one **generated skill** per configured agent.
- A **generated skill** is exposed as an **installed skill** via a directory symlink; its **supporting files** are reached through that symlink.
- A **skill template** contains one skill body and zero or more **supporting files**.

## Example dialogue

> **Dev:** "Where does `CONTEXT-FORMAT.md` end up — does the agent read it directly?"
> **Maintainer:** "It's a **supporting file** in the **skill template**. On generate it lands in the **generated skill**, and the agent reaches it through the **installed skill** symlink. The agent never touches the template."

## Flagged ambiguities

- "skill" was used for the authored source, the rendered output, and the symlink interchangeably — resolved: **skill template** (source) → **generated skill** (rendered) → **installed skill** (symlink).
- "sentinel" / "completion sentinel" named the `<pba:complete/>` line as if dispatch keyed on it, but dispatch keys on the `in-review` status. Resolved: **completion signal** (the status flip dispatch acts on) vs **completion marker** (the decorative stdout line). "sentinel" is retained only for unrelated internal uses — Go error sentinels (`IsHITLOnly`/`IsSetupFailure`) and the flock lock-sentinel file.
- "dispatcher" was ambiguous between the Go binary (the old `pb dispatch` cold-subprocess engine) and the live agent. Resolved: the **dispatcher** is the agent driving the harness **Workflow** tool (see ADR-0006); Go is the state store + git mechanics behind `pba merge`. The Go orchestration engine and `pb dispatch` are removed.
