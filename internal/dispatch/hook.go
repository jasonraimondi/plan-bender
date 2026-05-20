package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const hookDefaultTimeout = 10 * time.Minute

// RunHook executes cmd via `sh -c` with Cmd.Dir=dir, streams stdout to outWriter
// prefixed with "[hook] ", captures stderr, and returns the captured stderr +
// any error. An empty cmd is a no-op (no process spawned).
//
// ctx bounds the hook lifetime. If ctx has no deadline, RunHook applies
// hookDefaultTimeout. A timeout is reported as a context.DeadlineExceeded
// wrapped in the returned error.
func RunHook(ctx context.Context, cmd, dir string, outWriter io.Writer) (string, error) {
	if cmd == "" {
		return "", nil
	}
	if outWriter == nil {
		outWriter = os.Stdout
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, hookDefaultTimeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = dir
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		c.Stdin = devNull
		defer devNull.Close()
	}

	var stderrBuf bytes.Buffer
	c.Stderr = &stderrBuf

	// Setting c.Stdout (rather than calling StdoutPipe) lets os/exec own the
	// copy goroutine, so cmd.Wait + WaitDelay together remain the sole
	// synchronization point — no separate reader to outlive the pipe close.
	lw := &linePrefixWriter{prefix: "[hook] ", out: outWriter}
	c.Stdout = lw

	configureProcessGroup(c)
	c.WaitDelay = subprocessWaitDelay

	if err := c.Start(); err != nil {
		return "", fmt.Errorf("starting hook %q: %w", cmd, err)
	}

	waitErr := c.Wait()
	lw.Flush()

	stderr := strings.TrimRight(stderrBuf.String(), "\n")
	if waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return stderr, fmt.Errorf("hook %q timed out: %w (stderr: %s)", cmd, ctx.Err(), stderr)
		}
		return stderr, fmt.Errorf("hook %q exited non-zero: %w (stderr: %s)", cmd, waitErr, stderr)
	}
	return stderr, nil
}
