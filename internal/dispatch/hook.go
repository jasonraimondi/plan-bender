package dispatch

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// RunHook executes cmd via `sh -c` with Cmd.Dir=dir, streams stdout to outWriter
// prefixed with "[hook] ", captures stderr, and returns the captured stderr +
// any error. An empty cmd is a no-op (no process spawned).
func RunHook(cmd, dir string, outWriter io.Writer) (string, error) {
	if cmd == "" {
		return "", nil
	}
	if outWriter == nil {
		outWriter = os.Stdout
	}

	c := exec.Command("sh", "-c", cmd)
	c.Dir = dir
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		c.Stdin = devNull
		defer devNull.Close()
	}

	var stderrBuf bytes.Buffer
	c.Stderr = &stderrBuf
	stdout, err := c.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("attaching stdout: %w", err)
	}

	if err := c.Start(); err != nil {
		return "", fmt.Errorf("starting hook %q: %w", cmd, err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			fmt.Fprintln(outWriter, "[hook] "+scanner.Text())
		}
	}()

	waitErr := c.Wait()
	wg.Wait()

	stderr := strings.TrimRight(stderrBuf.String(), "\n")
	if waitErr != nil {
		return stderr, fmt.Errorf("hook %q exited non-zero: %w (stderr: %s)", cmd, waitErr, stderr)
	}
	return stderr, nil
}
