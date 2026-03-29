- archive plans to an optional second location
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
