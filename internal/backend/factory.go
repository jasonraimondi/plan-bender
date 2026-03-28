package backend

import (
	"context"
	"fmt"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

// New creates a Backend from config.
func New(ctx context.Context, cfg config.Config) (Backend, error) {
	switch cfg.Backend {
	case config.BackendYAMLFS:
		return NewYAMLFS(cfg), nil
	case config.BackendLinear:
		return NewLinear(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown backend: %q", cfg.Backend)
	}
}
