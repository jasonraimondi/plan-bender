package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandEnv_ResolvesEnvVars(t *testing.T) {
	t.Setenv("PB_TEST_API_KEY", "lin_api_secret123")
	t.Setenv("PB_TEST_TEAM", "ENG")

	cfg := &Config{
		Linear: LinearConfig{
			APIKey: "$PB_TEST_API_KEY",
			Team:   "$PB_TEST_TEAM",
		},
	}
	expandEnv(cfg)

	assert.Equal(t, "lin_api_secret123", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestExpandEnv_BraceSyntax(t *testing.T) {
	t.Setenv("PB_TEST_API_KEY", "lin_api_braces")

	cfg := &Config{
		Linear: LinearConfig{
			APIKey: "${PB_TEST_API_KEY}",
		},
	}
	expandEnv(cfg)

	assert.Equal(t, "lin_api_braces", cfg.Linear.APIKey)
}

func TestExpandEnv_LiteralValuesUnchanged(t *testing.T) {
	cfg := &Config{
		Linear: LinearConfig{
			APIKey: "lin_api_literal",
			Team:   "ENG",
		},
	}
	expandEnv(cfg)

	assert.Equal(t, "lin_api_literal", cfg.Linear.APIKey)
	assert.Equal(t, "ENG", cfg.Linear.Team)
}

func TestExpandEnv_UnsetVarBecomesEmpty(t *testing.T) {
	cfg := &Config{
		Linear: LinearConfig{
			APIKey: "$PB_NONEXISTENT_VAR_12345",
		},
	}
	expandEnv(cfg)

	assert.Equal(t, "", cfg.Linear.APIKey)
}
