//go:build linux

package connectedstartup

import (
	"errors"
	"fmt"
	"testing"
)

func TestStartupExitCodesAreClosedAndRetainRefusal(t *testing.T) {
	for _, code := range []int{73, 74, 75, 76} {
		err := &stageError{code}
		if ExitCode(fmt.Errorf("private detail: %w", err)) != code || !errors.Is(err, ErrUnsafe) || err.Error() != "connected_startup_refused" {
			t.Fatal("stage leaked or lost refusal")
		}
	}
	for _, err := range []error{nil, ErrUnsafe, errors.New("hlc_private"), &stageError{0}, &stageError{77}, &stageError{-1}} {
		if ExitCode(err) != 22 {
			t.Fatal("unknown diagnostic status accepted")
		}
	}
}
