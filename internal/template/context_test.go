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

func TestBuildContext_BackendOnlyPhaseHiddenWhenLinearDisabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Linear.Enabled = false
	ctx := BuildContext(cfg, config.ResolvedAgent{Name: "claude-code"})

	phases, _ := ctx["pipeline_phases"].([]map[string]string)
	for _, p := range phases {
		assert.NotEqual(t, "bender-sync-linear", p["skill"], "sync phase must be hidden when Linear is disabled")
	}
}

func TestBuildContext_BackendOnlyPhaseShownWhenLinearEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Linear.Enabled = true
	ctx := BuildContext(cfg, config.ResolvedAgent{Name: "claude-code"})

	phases, _ := ctx["pipeline_phases"].([]map[string]string)
	found := false
	for _, p := range phases {
		if p["skill"] == "bender-sync-linear" {
			found = true
			break
		}
	}
	assert.True(t, found, "sync phase must be present when Linear is enabled")
}

func TestSkillRequiresBackend(t *testing.T) {
	assert.True(t, SkillRequiresBackend("bender-sync-linear"))
	assert.False(t, SkillRequiresBackend("bender-write-prd"))
	assert.False(t, SkillRequiresBackend("does-not-exist"))
}
