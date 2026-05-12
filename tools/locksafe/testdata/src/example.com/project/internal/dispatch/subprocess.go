package dispatch

import (
	"context"

	"example.com/project/internal/planrepo"
)

func RunSubprocess(context.Context) {}

func SamePackageSubprocessWhileSessionLive(ctx context.Context) {
	sess := planrepo.Open()
	defer sess.Close()
	RunSubprocess(ctx) // want "PlanSession sess is live across subprocess call RunSubprocess"
}
