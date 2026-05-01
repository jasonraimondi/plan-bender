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

	cmd := exec.CommandContext(ctx, "claude", "--print", "--output-format", "stream-json", "-p", prompt)
	cmd.Dir = worktreePath
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		cmd.Stdin = devNull
		defer devNull.Close()
	}

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
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Fprintln(outWriter, prefix+line)
			logBuf.WriteString(line)
			logBuf.WriteByte('\n')
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

	reason := buildFailureReason(waitErr, postStatus, stderrText)
	res.Err = errors.New(reason)
	return mark(res, reason)
}

func buildFailureReason(waitErr error, postStatus, stderr string) string {
	switch {
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

// markBlocked persists the blocked status. Returns the SubResult so the caller
// can return it directly, plus any write error so the caller can surface it
// (rather than letting the dispatch loop see a "still todo" file and retry).
func markBlocked(res SubResult, plansDir, slug string, issue schema.IssueYaml, reason string) (SubResult, error) {
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

	store := backend.NewProdPlanStore(plansDir)
	res.Success = false
	if writeErr := store.WriteIssue(slug, current); writeErr != nil {
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
