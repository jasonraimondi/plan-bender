package backend

import (
	"context"

	"github.com/jasonraimondi/plan-bender/internal/config"
)

func New(ctx context.Context, root string, cfg config.Config) (Backend, error) {
	if cfg.Linear.Enabled {
		return NewLinear(ctx, root, cfg)
	}
	return NewLocalFS(cfg), nil
}
