# plan-bender

Domain language for plan-bender. This glossary captures terms specific to the project's contexts as they are resolved. It is a glossary, not a spec — no implementation details.

## Language

### Dispatch

**Integration branch**:
The branch dispatch builds work onto, named `<user>/<slug>`. Forked from the `base` (default branch unless `--base` is passed). Per-issue worktrees merge into it; dispatch never touches the parent repo's HEAD.
_Avoid_: feature branch, work branch, dispatch branch

**Landing branch**:
The branch the parent repo's HEAD is on when the operator invokes a dispatch skill. Distinct from `base` (where the integration branch forks from) and from the integration branch itself. Used as the merge destination when the operator picks the `merge` completion mode.
_Avoid_: target branch, starting branch, current branch (ambiguous), feature branch

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
