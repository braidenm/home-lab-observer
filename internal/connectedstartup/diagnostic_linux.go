//go:build linux

package connectedstartup

import "errors"

// Fixed terminal statuses distinguish refusal boundaries, never raw OS errors.
// They do not change Restart=no or grant activation authority.
type stageError struct{ code int }

func (e *stageError) Error() string { return "connected_startup_refused" }
func (e *stageError) Unwrap() error { return ErrUnsafe }

func ExitCode(err error) int {
	var failure *stageError
	if errors.As(err, &failure) {
		switch failure.code {
		case 73, 74, 75, 76:
			return failure.code
		}
	}
	return 22
}
