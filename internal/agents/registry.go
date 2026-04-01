package agents

import (
	"fmt"
	"sort"
)

// Scope defines where an agent's skills can be installed.
type Scope string

const (
	ProjectOnly   Scope = "project-only"
	UserOnly      Scope = "user-only"
	ProjectOrUser Scope = "project-or-user"
)

// AgentConfig describes a registered agent's skill directories and behavior.
type AgentConfig struct {
	Name             string
	ProjectDir       string
	UserDir          string
	Scope            Scope
	GitignorePattern string
}

var registry = map[string]AgentConfig{
	"claude-code": {
		Name:             "claude-code",
		ProjectDir:       ".claude/skills/",
		UserDir:          "~/.claude/skills/",
		Scope:            ProjectOrUser,
		GitignorePattern: ".claude/skills/bender-*",
	},
	"openclaw": {
		Name:    "openclaw",
		UserDir: "~/.openclaw/skills/",
		Scope:   UserOnly,
	},
	"opencode": {
		Name:             "opencode",
		ProjectDir:       ".opencode/skills/",
		UserDir:          "~/.config/opencode/skills/",
		Scope:            ProjectOrUser,
		GitignorePattern: ".opencode/skills/bender-*",
	},
	"pi": {
		Name:             "pi",
		ProjectDir:       ".pi/skills/",
		UserDir:          "~/.pi/agent/skills/",
		Scope:            ProjectOrUser,
		GitignorePattern: ".pi/skills/bender-*",
	},
}

// Get returns the AgentConfig for the given agent name, or an error if not found.
func Get(name string) (AgentConfig, error) {
	cfg, ok := registry[name]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent not found: %q", name)
	}
	return cfg, nil
}

// Names returns a sorted list of all registered agent names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
