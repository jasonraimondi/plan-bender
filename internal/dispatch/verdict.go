package dispatch

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// Outcome classifies the result of a sub-agent subprocess run.
type Outcome interface {
	Reason() string
	IsSuccess() bool
}

// Success means the subprocess exited 0 and the post-run issue file is in-review.
type Success struct{}

func (Success) Reason() string  { return "" }
func (Success) IsSuccess() bool { return true }

// ExitNonZero means the subprocess returned a non-zero exit (or otherwise
// failed to run). Code is -1 when Err is not an *exec.ExitError.
type ExitNonZero struct {
	Code int
	Err  error
}

func (e ExitNonZero) Reason() string {
	return fmt.Sprintf("subprocess exited with code %d: %v", e.Code, e.Err)
}
func (ExitNonZero) IsSuccess() bool { return false }

// WrongPostStatus means the subprocess exited 0 but the post-run issue file
// shows a status other than "in-review".
type WrongPostStatus struct {
	Actual string
}

func (w WrongPostStatus) Reason() string {
	return fmt.Sprintf("subprocess exited 0 but issue status is %s, expected in-review", w.Actual)
}
func (WrongPostStatus) IsSuccess() bool { return false }

// Unreadable means the post-run issue file could not be read.
type Unreadable struct {
	Err error
}

func (u Unreadable) Reason() string {
	return fmt.Sprintf("post-run issue file unreadable: %v", u.Err)
}
func (Unreadable) IsSuccess() bool { return false }

// Verdict classifies a subprocess run into one of four outcomes.
//
// Precedence (highest first):
//  1. loadErr != nil — Unreadable. A missing or corrupt post-run file beats every
//     other signal: without it we cannot know what the sub-agent did.
//  2. exitErr != nil — ExitNonZero. We trust the exit code over a stale issue file.
//  3. post != nil and post.Status != "in-review" — WrongPostStatus.
//  4. otherwise — Success.
func Verdict(exitErr error, loadErr error, post *schema.IssueYaml) Outcome {
	if loadErr != nil {
		return Unreadable{Err: loadErr}
	}
	if exitErr != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(exitErr, &ee) {
			code = ee.ExitCode()
		}
		return ExitNonZero{Code: code, Err: exitErr}
	}
	if post != nil && post.Status != "in-review" {
		return WrongPostStatus{Actual: post.Status}
	}
	return Success{}
}
