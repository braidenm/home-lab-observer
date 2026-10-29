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
	phase := "UNKNOWN"
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		refusal, ok := cause.(*lifecycleError)
		if !ok {
			continue
		}
		switch refusal.phase {
		case "REQUEST", "LEASE", "INSTALLED_STATE", "HOST_PROFILE", "PRINCIPALS", "SERVICE_STATE", "RUNTIME_GATE",
			"READER_SOCKET", "READER_STARTUP",
			"ACTIVATION_PREPARE", "ACTIVATION_CLEANUP", "UPLOADER_START", "UPLOADER_AUDIT", "REQUEST_PUBLICATION", "UPLOADER_RESPONSE",
			"COLLECTOR_START", "COLLECTOR_READY", "FINAL_AUDIT", "COMMIT_PUBLICATION", "UPLOADER_READY":
			phase = refusal.phase
		default:
			phase = "UNKNOWN"
		}
	}
	return phase
}
