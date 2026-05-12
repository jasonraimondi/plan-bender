package backendlive

import (
	"context"

	"example.com/project/internal/backend"
	"example.com/project/internal/planrepo"
)

func BackendCallWhileSessionLive(ctx context.Context, be backend.Backend) error {
	sess := planrepo.Open()
	defer sess.Close()
	return be.PullProject(ctx, "ship") // want "PlanSession sess is live across backend call be.PullProject"
}

func BackendCallAfterClose(ctx context.Context, be backend.Backend) error {
	sess := planrepo.Open()
	if err := sess.Close(); err != nil {
		return err
	}
	return be.PullProject(ctx, "ship")
}
