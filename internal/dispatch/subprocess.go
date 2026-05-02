package dispatch

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jasonraimondi/plan-bender/internal/backend"
	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// SubResult is the outcome of a single sub-agent subprocess.
type SubResult struct {
	IssueID int
	Success bool
	Branch  string
	Err     error
}

// RunSubprocess executes one `claude --print` invocation in worktreePath, streams
// its stdout to outWriter prefixed with [issue-N], and returns a SubResult based
// on the post-run YAML status.
//
// plansDir is the absolute path to the parent repo's plans dir. logDir receives
// the full output transcript at logDir/{id}.log.
func RunSubprocess(
	ctx context.Context,
	slug string,
	issue schema.IssueYaml,
	prompt, worktreePath, plansDir, logDir string,
	outWriter io.Writer,
) SubResult {
	res := SubResult{IssueID: issue.ID}

	if outWriter == nil {
		outWriter = os.Stdout
	}

	mark := func(r SubResult, reason string) SubResult {
		out, err := markBlocked(r, plansDir, slug, issue, reason)
		if err != nil {
			fmt.Fprintf(outWriter, "[issue-%d] warning: failed to persist blocked status: %v\n", issue.ID, err)
		}
		return out
	}

	// Pass the prompt on stdin rather than `-p <prompt>`. The skill body begins
	// with `---` (YAML frontmatter), and claude's flag parser rejects -p values
	// that look like options.
	cmd := exec.CommandContext(ctx, "claude", "--print", "--verbose", "--output-format", "stream-json")
	cmd.Dir = worktreePath
	cmd.Stdin = strings.NewReader(prompt)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		res.Err = fmt.Errorf("attaching stdout pipe: %w", err)
		return mark(res, res.Err.Error())
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			res.Err = fmt.Errorf("claude binary not found in PATH; install claude or set PATH before dispatch")
		} else {
			res.Err = fmt.Errorf("starting claude: %w", err)
		}
		return mark(res, res.Err.Error())
	}

	prefix := fmt.Sprintf("[issue-%d] ", issue.ID)
	var logBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// bufio.Reader (not Scanner) so a single stream-json event embedding a
		// large tool result can exceed any fixed buffer cap.
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				stripped := strings.TrimRight(line, "\n")
				fmt.Fprintln(outWriter, prefix+stripped)
				logBuf.WriteString(stripped)
				logBuf.WriteByte('\n')
			}
			if err != nil {
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	stderrText := stderrBuf.String()

	if logDir != "" {
		if err := writeLog(logDir, issue.ID, logBuf.Bytes(), []byte(stderrText)); err != nil {
			fmt.Fprintf(outWriter, "%swarning: failed to write log: %v\n", prefix, err)
		}
	}

	postIssue, readErr := loadIssue(plansDir, slug, issue.ID)
	postStatus := ""
	if readErr == nil {
		postStatus = postIssue.Status
	}

	if waitErr == nil && postStatus == "in-review" {
		res.Success = true
		return res
	}

	reason := buildFailureReason(ctx, waitErr, postStatus, stderrText)
	res.Err = errors.New(reason)
	return mark(res, reason)
}

func buildFailureReason(ctx context.Context, waitErr error, postStatus, stderr string) string {
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	stderr = truncateForNotes(stderr)
	switch {
	case timedOut && stderr != "":
		return fmt.Sprintf("subprocess timed out: %v\n%s", waitErr, stderr)
	case timedOut:
		return fmt.Sprintf("subprocess timed out: %v", waitErr)
	case waitErr != nil && stderr != "":
		return fmt.Sprintf("subprocess failed: %v\n%s", waitErr, stderr)
	case waitErr != nil:
		return fmt.Sprintf("subprocess failed: %v", waitErr)
	case postStatus == "":
		return "subprocess exited 0 but issue file is unreadable; treating as failure"
	default:
		return fmt.Sprintf("subprocess exited 0 but issue status is %q (expected in-review)", postStatus)
	}
}

// stderrNotesLimit caps how much stderr we embed in an issue's notes on failure.
// The full transcript still lands in the dispatch log file; the cap exists so
// a verbose subprocess error (e.g. an entire skill body echoed back as an
// "unknown option" message) cannot bloat the YAML.
const stderrNotesLimit = 2048

func truncateForNotes(s string) string {
	if len(s) <= stderrNotesLimit {
		return s
	}
	return s[:stderrNotesLimit] + "\n... (truncated; see dispatch log for full output)"
}

// markBlocked persists the blocked status. Returns the SubResult so the caller
// can return it directly, plus any write error so the caller can surface it
// (rather than letting the dispatch loop see a "still todo" file and retry).
func markBlocked(res SubResult, plansDir, slug string, issue schema.IssueYaml, reason string) (SubResult, error) {
	res.Success = false

	release, err := backend.LockPlanDir(plansDir)
	if err != nil {
		return res, fmt.Errorf("locking plans dir to mark issue #%d blocked: %w", issue.ID, err)
	}
	defer release()

	current, err := loadIssue(plansDir, slug, issue.ID)
	if err != nil {
		current = &issue
	}

	current.Status = "blocked"
	current.Updated = time.Now().Format("2006-01-02")
	if current.Notes == nil {
		current.Notes = &reason
	} else {
		merged := *current.Notes + "\n\n" + reason
		current.Notes = &merged
	}

	if writeErr := backend.NewUnlockedPlanStore(plansDir).WriteIssue(slug, current); writeErr != nil {
		return res, fmt.Errorf("writing blocked status for issue #%d: %w", issue.ID, writeErr)
	}
	return res, nil
}

func loadIssue(plansDir, slug string, id int) (*schema.IssueYaml, error) {
	issues, err := plan.LoadIssues(plansDir, slug)
	if err != nil {
		return nil, err
	}
	for i := range issues {
		if issues[i].ID == id {
			return &issues[i], nil
		}
	}
	return nil, fmt.Errorf("issue #%d not found in %q", id, slug)
}

func writeLog(logDir string, id int, stdout, stderr []byte) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(logDir, fmt.Sprintf("%d.log", id))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

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
