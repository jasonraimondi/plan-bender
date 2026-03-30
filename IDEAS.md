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

## Multi-agent support (make plan-bender generic)

**Problem:** `install_target` conflates install scope (project vs user) with agent target — both resolve to `.claude/skills/`. Claude Code idioms are also baked into templates.

**Proposed change — two layers:**

### Layer 1: Split scope from agent target

Replace `install_target: project | user` with two fields:

```yaml
install_scope: project | user       # where (repo-local vs global)
agent_target: claude-code | openclaw | codex | gemini-cli | cursor
```

Each `agent_target` owns its skill directory path:
- `claude-code` → `.claude/skills/` (project) or `~/.claude/skills/` (user)
- `openclaw`    → `~/.openclaw/workspace/skills/` (always user-global)
- etc.

Gitignore entries also become agent-target-aware.

### Layer 2: Agent-aware templates

SKILL.md frontmatter is already cross-agent (AgentSkills spec). Template content is mostly generic too. Main exceptions are agent-specific tool names (e.g. `AskUserQuestionTool` for Claude Code) — these can be either template variables or removed entirely.

**Net result:** `pb install --agent openclaw` drops skills into the right place and they work natively in any supported agent.
