package template

import (
	"testing"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestBuildContext_ExtraKeysSurfaced(t *testing.T) {
	cfg := config.Defaults()
	agent := config.ResolvedAgent{
		Name:  "claude-code",
		Extra: map[string]any{"question_tool": "AskUserQuestion"},
	}
	ctx := BuildContext(cfg, agent)
	assert.Equal(t, "AskUserQuestion", ctx["question_tool"])
}

func TestBuildContext_BuiltinWinsOnCollision_Agent(t *testing.T) {
	cfg := config.Defaults()
	agent := config.ResolvedAgent{
		Name:  "claude-code",
		Extra: map[string]any{"agent": "intruder"},
	}
	ctx := BuildContext(cfg, agent)
	assert.Equal(t, "claude-code", ctx["agent"])
}

func TestBuildContext_BuiltinWinsOnCollision_PlansDir(t *testing.T) {
	cfg := config.Defaults()
	agent := config.ResolvedAgent{
		Name:  "claude-code",
		Extra: map[string]any{"plans_dir": "injected"},
	}
	ctx := BuildContext(cfg, agent)
	assert.Equal(t, cfg.PlansDir, ctx["plans_dir"])
}

func TestBuildContext_NoExtraKeysIsClean(t *testing.T) {
	cfg := config.Defaults()
	agent := config.ResolvedAgent{Name: "claude-code"}
	ctx := BuildContext(cfg, agent)
	assert.Equal(t, "claude-code", ctx["agent"])
	assert.NotNil(t, ctx["plans_dir"])
}
