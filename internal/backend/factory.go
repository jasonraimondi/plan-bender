package backend

import (
	"context"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

// New creates a Backend from config.
func New(ctx context.Context, cfg config.Config) (Backend, error) {
	if cfg.Linear.Enabled {
		return NewLinear(ctx, cfg)
	}
	return NewYAMLFS(cfg), nil
}
