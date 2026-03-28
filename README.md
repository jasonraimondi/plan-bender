# plan-bender

A framework + CLI for configurable, template-driven planning with pluggable tracking backends. Designed for AI-agent workflows (Claude Code skills) but works standalone.

## What it does

plan-bender manages a planning pipeline — from PRD to issues to implementation — using YAML files as the source of truth. It generates Claude Code skills from configurable templates, so the same planning workflow adapts to different projects with different tracks, workflow states, and backends.

```
Interview → Write PRD → Break into Issues → Review → Implement → Archive
```

## Install

```bash
npm install -g plan-bender   # or use npx
```

Requires Node >= 22.

## Quick start

```bash
plan-bender init              # interactive setup, writes plan-bender.yaml
plan-bender install           # symlink skills to ~/.claude/skills/
```

Then in Claude Code: `/planning-write-a-prd`, `/planning-prd-to-issues`, `/implement-prd`, etc.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Interactive project setup — prompts for backend, tracks, points, generates skills |
| `generate-skills` | Render all `.skill.tmpl` templates with project config |
| `install` | Symlink generated skills to `~/.claude/skills/` |
| `validate <slug>` | Schema validation: PRD fields, issue fields, cross-refs, cycle detection |
| `write-prd <slug>` | Validate and atomically write a PRD YAML (stdin or `--file`) |
| `write-issue <slug> <id>` | Validate and atomically write an issue YAML |
| `sync <slug>` | Push local state to configured backend (or `--pull` to pull) |
| `status [slug]` | Terminal dashboard — per-plan or cross-plan, with track coverage |
| `graph <slug>` | Mermaid dependency DAG from `blocked_by` edges |
| `archive <slug>` | Move completed plan to `.archive/` with summary |

## Configuration

Three-layer config with deep merge (later layers win):

1. **Global**: `~/.config/plan-bender/defaults.yaml`
2. **Project**: `plan-bender.yaml` (committed)
3. **Local**: `plan-bender.local.yaml` (gitignored, for secrets)

```yaml
# plan-bender.yaml
backend: yaml-fs          # yaml-fs | linear
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
step_pattern: "Target — behavior"
plans_dir: ./plans/
max_points: 3
pipeline:
  skip: []                # skill names to exclude from generation
issue_schema:
  custom_fields: []       # [{name, type, required, enum_values}]
linear:
  api_key: ...            # put in plan-bender.local.yaml
  team: ...
  status_map: {}          # local status → Linear state name
```

## Plan structure

```
plans/
  my-project/
    prd.yaml              # PRD with use cases, decisions, risks
    issues/
      1-setup-auth.yaml   # issue with track, status, steps, blocked_by
      2-add-tokens.yaml
      ...
```

## Backends

- **yaml-fs** (default): everything in local YAML files
- **linear**: syncs issues to Linear via `@linear/sdk` with bidirectional status mapping

## Template engine

Skills are generated from `.skill.tmpl` files using a custom template engine:

- `${var}` / `${var.nested}` — variable interpolation
- `${var | upper}`, `${var | kebab}`, `${var | join(, )}`, `${var | indent(4)}` — pipe transforms
- `@if condition` / `@if !condition` ... `@end` — conditional blocks
- `@each items as item` ... `@end` — iteration

Override bundled templates by placing `.skill.tmpl` files in `.plan-bender/templates/`.

## Development

```bash
pnpm install
pnpm build        # tsdown → dist/cli.mjs
pnpm test         # vitest
pnpm dev          # watch mode
pnpm lint         # oxlint
pnpm check        # tsc --noEmit
```

## License

MIT
