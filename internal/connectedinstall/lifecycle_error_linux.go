//go:build linux

package connectedinstall

import "errors"

type lifecycleError struct {
	phase string
	cause error
}

func (e *lifecycleError) Error() string { return "connected_lifecycle_refused" }
func (e *lifecycleError) Unwrap() error { return e.cause }

// LifecyclePhase exposes only code-owned categories, never a wrapped OS error.
func LifecyclePhase(err error) string {
	var refusal *lifecycleError
	if errors.As(err, &refusal) {
		switch refusal.phase {
		case "REQUEST", "LEASE", "INSTALLED_STATE", "HOST_PROFILE", "PRINCIPALS", "SERVICE_STATE", "RUNTIME_GATE":
			return refusal.phase
		}
	}
	return "UNKNOWN"
}
