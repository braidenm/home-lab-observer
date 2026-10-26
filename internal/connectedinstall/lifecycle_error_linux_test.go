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
	for _, phase := range []string{"ACTIVATION_PREPARE", "ACTIVATION_CLEANUP", "UPLOADER_START", "UPLOADER_AUDIT", "REQUEST_PUBLICATION", "UPLOADER_RESPONSE", "COLLECTOR_START", "COLLECTOR_READY", "FINAL_AUDIT", "COMMIT_PUBLICATION", "UPLOADER_READY"} {
		inner := &lifecycleError{phase, ErrRecovery}
		if LifecyclePhase(&lifecycleError{"RUNTIME_GATE", inner}) != phase || !errors.Is(inner, ErrRecovery) {
			t.Fatal("specific refusal hidden by outer transaction")
		}
	}
	if LifecyclePhase(&lifecycleError{"RUNTIME_GATE", &lifecycleError{"hlc_private", ErrUnsafe}}) != "UNKNOWN" {
		t.Fatal("unknown nested stage did not fail closed")
	}
}
