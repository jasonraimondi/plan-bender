package status

import (
	"errors"
	"fmt"
)

// ErrAlreadyInState signals an idempotent no-op: current state already equals
// the requested target. The Owner emits no audit log and writes nothing.
// Compare with errors.Is.
var ErrAlreadyInState = errors.New("issue already in target state")

// ErrCASMismatch signals current state is neither in the from-set nor equal
// to the target. Carries the actual current state so callers can decide
// whether to bail, retry, or resurface. Extract via errors.As.
type ErrCASMismatch struct {
	Current Status
}

func (e *ErrCASMismatch) Error() string {
	return fmt.Sprintf("CAS mismatch: current state is %q", e.Current)
}

// ErrIllegalTransition signals (current → to) is not in the allowed-transitions
// table — i.e. the workflow forbids this edge regardless of CAS. Extract
// via errors.As.
type ErrIllegalTransition struct {
	From Status
	To   Status
}

func (e *ErrIllegalTransition) Error() string {
	return fmt.Sprintf("illegal transition: %q → %q", e.From, e.To)
}
