- archive plans to an optional second location

## Two CLIs: `pb` for humans, `pba` (or `pb-agent`) for agents

**Problem:** `pb` is implicitly human-oriented — interactive prompts, pretty output, gitignore side effects. Agents can't drive it cleanly without workarounds.

**Proposal:** Two thin CLI entrypoints sharing the same core logic:

- **`pb`** — human CLI. Interactive, pretty tables, opinionated defaults. What you run in a terminal.
- **`pba`** (or `pb-agent`) — agent CLI. Always JSON output, never interactive, structured errors with machine-readable `code` fields, non-zero exits on failure. No TTY assumptions.

**Why two binaries over flags:** Agents shouldn't need to remember `--json --no-interactive` on every call. The contract is implicit in which binary you're invoking. No footguns.

**Implementation:** Two `main.go` entrypoints, shared `internal/` logic. Go makes this cheap. The agent binary can also expose agent-only commands that make no sense for humans:
- `pba context <slug>` — dump full plan context (PRD + open issues + status) as JSON for agent consumption
- `pba write-prd`, `pba write-issue` — validate + write, return result as JSON

**Net result:** Agents get a purpose-built interface. Humans keep the current UX. One codebase.
- TODO: create jasonraimondi/homebrew-tap repo and re-enable brew formula in .goreleaser.yaml

## Multi-agent support — SHIPPED

Implemented in the `multi-agent` PRD. Two binaries (`plan-bender` + `plan-bender-agent`), hardcoded agent registry (claude-code, openclaw), agent-aware templates with `{{.agent}}` conditionals, `pb setup` replaces init+install.
