package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/planrepo"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
)

// subprocessWaitDelay bounds how long cmd.Wait blocks after the process exits
// or after a ctx-cancel kill: once it elapses, os/exec force-kills the process
// and closes the pipe fds it owns, unblocking the I/O-copy goroutine even if a
// surviving grandchild still holds the pipe's write end. Shared by RunSubprocess
// and RunHook.
const subprocessWaitDelay = 10 * time.Second

// blockTransitionTimeout bounds the failure-path blocked-status write. The
// transition uses a fresh ctx (detached from any caller-supplied deadline) so
// a subprocess_timeout SIGKILL — or a Ctrl-C canceling the dispatch loop —
// cannot drop the write and leave the issue in-progress for the next loop to
// re-pick.
const blockTransitionTimeout = 30 * time.Second

// SubResult is the outcome of a single sub-agent subprocess.
type SubResult struct {
	IssueID int
	Success bool
	Branch  string
	Err     error
}

// RunSubprocess executes one `claude --print` invocation in worktreePath, streams
// its stdout to outWriter prefixed with [issue-N], and routes the post-Wait
// state through Verdict. On Success it returns SubResult with Success=true; on
// any other Outcome it transitions the issue to blocked with Outcome.Reason()
// and returns Success=false with Err carrying the reason.
//
// plans is the shared planrepo handle; the post-run loadIssue read flows
// through it. logDir receives the full output transcript at logDir/{id}.log.
func RunSubprocess(
	ctx context.Context,
	owner *status.Owner,
	plans *planrepo.Plans,
	slug string,
	issue schema.Issue,
	prompt, worktreePath, logDir string,
	outWriter io.Writer,
) SubResult {
	res := SubResult{IssueID: issue.ID}

	if outWriter == nil {
		outWriter = os.Stdout
	}

	block := func(reason string) SubResult {
		res.Success = false
		res.Err = errors.New(reason)
		txCtx, cancel := context.WithTimeout(context.Background(), blockTransitionTimeout)
		defer cancel()
		err := owner.Transition(txCtx, slug, issue.ID,
			blockFromStatuses,
			status.StatusBlocked, reason)
		if err != nil && !errors.Is(err, status.ErrAlreadyInState) {
			fmt.Fprintf(outWriter, "[issue-%d] warning: failed to persist blocked status: %v\n", issue.ID, err)
		}
		return res
	}

	// Pass the prompt on stdin rather than `-p <prompt>`. The skill body begins
	// with `---` (YAML frontmatter), and claude's flag parser rejects -p values
	// that look like options.
	cmd := exec.CommandContext(ctx, "claude", "--print", "--verbose", "--output-format", "stream-json")
	cmd.Dir = worktreePath
	cmd.Stdin = strings.NewReader(prompt)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	prefix := fmt.Sprintf("[issue-%d] ", issue.ID)
	var logBuf bytes.Buffer
	// Setting cmd.Stdout (rather than calling StdoutPipe) lets os/exec own the
	// copy goroutine. cmd.Wait then waits for that goroutine before closing the
	// pipe — so the tail can't be lost to a Wait/reader ordering race, and
	// WaitDelay still bounds the whole sequence.
	lw := &linePrefixWriter{prefix: prefix, out: outWriter, log: &logBuf}
	cmd.Stdout = lw

	configureProcessGroup(cmd)
	cmd.WaitDelay = subprocessWaitDelay

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return block("claude binary not found in PATH; install claude or set PATH before dispatch")
		}
		return block(fmt.Sprintf("starting claude: %v", err))
	}

	waitErr := cmd.Wait()
	lw.Flush()

	stderrText := stderrBuf.String()

	if logDir != "" {
		if err := writeLog(logDir, issue.ID, logBuf.Bytes(), []byte(stderrText)); err != nil {
			fmt.Fprintf(outWriter, "%swarning: failed to write log: %v\n", prefix, err)
		}
	}

	post, loadErr := loadIssue(plans, slug, issue.ID)

	// Wrap waitErr with stderr so the persisted blocked-state note retains
	// observability. %w preserves the unwrap chain so Verdict's errors.As
	// against *exec.ExitError still recovers the exit code. Cap the stderr
	// portion so a verbose subprocess error cannot bloat the issue JSON.
	exitErr := waitErr
	if exitErr != nil {
		// A SIGKILL from exec.CommandContext's deadline is otherwise
		// indistinguishable from an OS OOM-kill — both surface as
		// "signal: killed" with exit code -1. Naming the subprocess_timeout
		// here gives the operator an actionable knob instead of a guess.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			exitErr = fmt.Errorf("subprocess timed out (exceeded subprocess_timeout); SIGKILL sent: %w", waitErr)
		}
		if stderr := strings.TrimSpace(stderrText); stderr != "" {
			exitErr = fmt.Errorf("%w\n%s", exitErr, truncateForNotes(stderr))
		}
	}

	outcome := Verdict(exitErr, loadErr, post)
	if outcome.IsSuccess() {
		res.Success = true
		return res
	}
	return block(outcome.Reason())
}

// stderrNotesLimit caps how much stderr we embed in an issue's notes on failure.
// The full transcript still lands in the dispatch log file; the cap exists so
// a verbose subprocess error (e.g. an entire skill body echoed back as an
// "unknown option" message) cannot bloat the JSON.
const stderrNotesLimit = 2048

func truncateForNotes(s string) string {
	if len(s) <= stderrNotesLimit {
		return s
	}
	return s[:stderrNotesLimit] + "\n... (truncated; see dispatch log for full output)"
}

func loadIssue(plans *planrepo.Plans, slug string, id int) (*schema.Issue, error) {
	sess, err := plans.Open(slug)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	for i := range sess.Snapshot().Issues {
		if sess.Snapshot().Issues[i].ID == id {
			iss := sess.Snapshot().Issues[i]
			return &iss, nil
		}
	}
	return nil, fmt.Errorf("issue #%d not found in %q", id, slug)
}

// separatorPrefix marks the start of one dispatch run inside a per-issue log.
// writeLog appends rather than truncates, so a re-dispatched issue keeps its
// prior runs; each run is delimited by a separator carrying a UTC timestamp.
const separatorPrefix = "=== dispatch run "

func writeLog(logDir string, id int, stdout, stderr []byte) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(logDir, fmt.Sprintf("%d.log", id))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	header := separatorPrefix + time.Now().UTC().Format(time.RFC3339) + " ===\n"
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	if _, err := f.Write(stdout); err != nil {
		return err
	}
	if len(stderr) > 0 {
		if _, err := f.WriteString("--- stderr ---\n"); err != nil {
			return err
		}
		if _, err := f.Write(stderr); err != nil {
			return err
		}
	}
	return nil
}
