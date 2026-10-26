//go:build linux

package connectedinstall

import (
	"errors"
	"strings"
	"testing"
)

func TestLifecyclePhaseIsClosedAndPreservesRecoveryClassification(t *testing.T) {
	for _, phase := range []string{"REQUEST", "LEASE", "INSTALLED_STATE", "HOST_PROFILE", "PRINCIPALS", "SERVICE_STATE", "RUNTIME_GATE"} {
		err := &lifecycleError{phase, ErrRecovery}
		if LifecyclePhase(err) != phase || !errors.Is(err, ErrRecovery) || strings.Contains(err.Error(), phase) {
			t.Fatal("fixed diagnostic contract failed")
		}
	}
	if LifecyclePhase(&lifecycleError{"hlc_private", errors.New("private detail")}) != "UNKNOWN" || LifecyclePhase(ErrUnsafe) != "UNKNOWN" {
		t.Fatal("unrecognized phase leaked")
	}
}
