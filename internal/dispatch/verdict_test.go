package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jasonraimondi/plan-bender/internal/schema"
)

// runForExitErr runs `sh -c "exit N"` and returns the resulting error, which
// wraps a real *exec.ExitError. Used so tests exercise the real errors.As
// extraction path inside Verdict.
func runForExitErr(t *testing.T, code int) error {
	t.Helper()
	err := exec.CommandContext(context.Background(), "sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	require.Error(t, err)
	var ee *exec.ExitError
	require.True(t, errors.As(err, &ee), "expected *exec.ExitError, got %T", err)
	return err
}

func TestVerdict(t *testing.T) {
	inReview := &schema.Issue{Status: "in-review"}
	inProgress := &schema.Issue{Status: "in-progress"}
	todo := &schema.Issue{Status: "todo"}

	exit1 := runForExitErr(t, 1)
	exit137 := runForExitErr(t, 137)
	nonExitErr := errors.New("not an exec.ExitError")
	loadErr := errors.New("permission denied")

	cases := []struct {
		name      string
		exitErr   error
		loadErr   error
		post      *schema.Issue
		want      Outcome
		reasonHas []string
	}{
		{
			name: "clean success",
			post: inReview,
			want: Success{},
		},
		{
			name:      "exit code 1",
			exitErr:   exit1,
			want:      ExitNonZero{Code: 1, Err: exit1},
			reasonHas: []string{"code 1"},
		},
		{
			name:      "exit code 137",
			exitErr:   exit137,
			want:      ExitNonZero{Code: 137, Err: exit137},
			reasonHas: []string{"code 137"},
		},
		{
			name:      "non-ExitError exit error gets code -1",
			exitErr:   nonExitErr,
			want:      ExitNonZero{Code: -1, Err: nonExitErr},
			reasonHas: []string{"-1", "not an exec.ExitError"},
		},
		{
			name:      "load error",
			loadErr:   loadErr,
			want:      Unreadable{Err: loadErr},
			reasonHas: []string{"unreadable", "permission denied"},
		},
		{
			name:      "exit success but status in-progress",
			post:      inProgress,
			want:      WrongPostStatus{Actual: "in-progress"},
			reasonHas: []string{"in-progress", "in-review"},
		},
		{
			name:      "exit success but status todo",
			post:      todo,
			want:      WrongPostStatus{Actual: "todo"},
			reasonHas: []string{"todo", "in-review"},
		},
		{
			name:      "loadErr beats exitErr",
			exitErr:   exit1,
			loadErr:   loadErr,
			want:      Unreadable{Err: loadErr},
			reasonHas: []string{"unreadable", "permission denied"},
		},
		{
			name:    "exitErr beats wrong status",
			exitErr: exit1,
			post:    inProgress,
			want:    ExitNonZero{Code: 1, Err: exit1},
		},
		{
			// The sub-agent ran `pba complete` (status -> in-review) and was
			// then SIGKILLed during wrap-up. Finished work must not be
			// downgraded to blocked by the post-completion kill.
			name:    "in-review post-status beats exitErr",
			exitErr: exit137,
			post:    inReview,
			want:    Success{},
		},
		{
			name:    "loadErr beats both exitErr and wrong status",
			exitErr: exit1,
			loadErr: loadErr,
			post:    inProgress,
			want:    Unreadable{Err: loadErr},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Verdict(tc.exitErr, tc.loadErr, tc.post)
			assert.Equal(t, tc.want, got)

			if _, ok := tc.want.(Success); ok {
				assert.True(t, got.IsSuccess())
			} else {
				assert.False(t, got.IsSuccess())
				for _, sub := range tc.reasonHas {
					assert.Contains(t, got.Reason(), sub)
				}
			}
		})
	}
}
