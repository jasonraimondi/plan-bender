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

	"github.com/jasonraimondi/plan-bender/internal/plan"
	"github.com/jasonraimondi/plan-bender/internal/schema"
	"github.com/jasonraimondi/plan-bender/internal/status"
)

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
// plansDir is the absolute path to the parent repo's plans dir. logDir receives
// the full output transcript at logDir/{id}.log.
func RunSubprocess(
	ctx context.Context,
	owner *status.Owner,
	slug string,
	issue schema.IssueYaml,
	prompt, worktreePath, plansDir, logDir string,
	outWriter io.Writer,
) SubResult {
	res := SubResult{IssueID: issue.ID}

	if outWriter == nil {
		outWriter = os.Stdout
	}

	block := func(reason string) SubResult {
		res.Success = false
		res.Err = errors.New(reason)
		err := owner.Transition(ctx, slug, issue.ID,
			[]status.Status{status.StatusTodo, status.StatusInProgress, status.StatusInReview},
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return block(fmt.Sprintf("attaching stdout pipe: %v", err))
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return block("claude binary not found in PATH; install claude or set PATH before dispatch")
		}
		return block(fmt.Sprintf("starting claude: %v", err))
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

	post, loadErr := loadIssue(plansDir, slug, issue.ID)

	// Wrap waitErr with stderr so the persisted blocked-state note retains
	// observability. %w preserves the unwrap chain so Verdict's errors.As
	// against *exec.ExitError still recovers the exit code. Cap the stderr
	// portion so a verbose subprocess error cannot bloat the issue YAML.
	exitErr := waitErr
	if exitErr != nil && strings.TrimSpace(stderrText) != "" {
		exitErr = fmt.Errorf("%w\n%s", waitErr, truncateForNotes(strings.TrimSpace(stderrText)))
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
// "unknown option" message) cannot bloat the YAML.
const stderrNotesLimit = 2048

func truncateForNotes(s string) string {
	if len(s) <= stderrNotesLimit {
		return s
	}
	return s[:stderrNotesLimit] + "\n... (truncated; see dispatch log for full output)"
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
