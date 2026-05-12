package subprocess

import (
	"context"
	"os/exec"

	"example.com/project/internal/dispatch"
	"example.com/project/internal/planrepo"
)

func ExecRunWhileSessionLive(ctx context.Context) error {
	sess := planrepo.Open()
	defer sess.Close()
	cmd := exec.CommandContext(ctx, "claude", "--print")
	return cmd.Run() // want "PlanSession sess is live across subprocess call cmd.Run"
}

func ExecWaitWhileSessionLive(ctx context.Context) error {
	sess := planrepo.Open()
	defer sess.Close()
	cmd := exec.CommandContext(ctx, "claude", "--print")
	if err := cmd.Start(); err != nil { // want "PlanSession sess is live across subprocess call cmd.Start"
		return err
	}
	return cmd.Wait() // want "PlanSession sess is live across subprocess call cmd.Wait"
}

func DispatchSubprocessWhileSessionLive(ctx context.Context) {
	sess := planrepo.Open()
	defer sess.Close()
	dispatch.RunSubprocess(ctx) // want "PlanSession sess is live across subprocess call dispatch.RunSubprocess"
}

func ExecRunAfterClose(ctx context.Context) error {
	sess := planrepo.Open()
	if err := sess.Close(); err != nil {
		return err
	}
	return exec.CommandContext(ctx, "true").Run()
}
