# plan-bender

> Structured planning pipeline for AI coding agents — from interview to implementation

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

Then in your agent:

```
/bender-orchestrator
```

The orchestrator reads your plan state and suggests the next action.

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

## Docs

- [CLI reference](docs/cli.md)
- [Configuration](docs/configuration.md)
- [Schema](docs/schema.md)

## License

MIT
