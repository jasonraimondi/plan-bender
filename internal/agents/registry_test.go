package agents_test

import (
	"slices"
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name             string
		agentName        string
		wantProjectDir   string
		wantUserDir      string
		wantScope        agents.Scope
		wantGitignore    string
		hasGitignore     bool
	}{
		{
			name:           "claude-code returns correct config",
			agentName:      "claude-code",
			wantProjectDir: ".claude/skills/",
			wantUserDir:    "~/.claude/skills/",
			wantScope:      agents.ProjectOrUser,
			wantGitignore:  ".claude/skills/bender-*",
			hasGitignore:   true,
		},
		{
			name:         "openclaw returns correct config",
			agentName:    "openclaw",
			wantUserDir:  "~/.openclaw/skills/",
			wantScope:    agents.UserOnly,
			hasGitignore: false,
		},
		{
			name:           "opencode returns correct config",
			agentName:      "opencode",
			wantProjectDir: ".opencode/skills/",
			wantUserDir:    "~/.config/opencode/skills/",
			wantScope:      agents.ProjectOrUser,
			wantGitignore:  ".opencode/skills/bender-*",
			hasGitignore:   true,
		},
		{
			name:           "pi returns correct config",
			agentName:      "pi",
			wantProjectDir: ".pi/skills/",
			wantUserDir:    "~/.pi/agent/skills/",
			wantScope:      agents.ProjectOrUser,
			wantGitignore:  ".pi/skills/bender-*",
			hasGitignore:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := agents.Get(tt.agentName)
			require.NoError(t, err)

			assert.Equal(t, tt.agentName, cfg.Name)
			assert.Equal(t, tt.wantProjectDir, cfg.ProjectDir)
			assert.Equal(t, tt.wantUserDir, cfg.UserDir)
			assert.Equal(t, tt.wantScope, cfg.Scope)

			if tt.hasGitignore {
				assert.Equal(t, tt.wantGitignore, cfg.GitignorePattern)
			} else {
				assert.Empty(t, cfg.GitignorePattern)
			}
		})
	}
}

func TestGet_UnknownAgent(t *testing.T) {
	_, err := agents.Get("unknown")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown")
}

func TestNames(t *testing.T) {
	names := agents.Names()

	assert.Contains(t, names, "claude-code")
	assert.Contains(t, names, "openclaw")
	assert.Contains(t, names, "opencode")
	assert.Contains(t, names, "pi")
	assert.Len(t, names, 4)

	assert.True(t, slices.IsSorted(names), "Names() should return sorted names")
}
