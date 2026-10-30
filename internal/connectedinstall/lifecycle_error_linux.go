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
			"READER_REQUEST", "READER_STATE", "READER_ACTIVATION_PIN", "READER_RESPONSE_CLEAR", "READER_SERVICE_START",
			"READER_AUDIT_BEFORE_REQUEST", "READER_REQUEST_BUILD", "READER_REQUEST_PUBLICATION", "READER_RESPONSE_WAIT",
			"READER_RESPONSE_MATCH", "READER_AUDIT_AFTER_RESPONSE", "READER_COMMIT_PUBLICATION", "READER_STATUS_WAIT", "READER_FINAL_IDENTITY",
			"ACTIVATION_PREPARE", "ACTIVATION_CLEANUP", "UPLOADER_START", "UPLOADER_AUDIT", "REQUEST_PUBLICATION", "UPLOADER_RESPONSE",
			"COLLECTOR_START", "COLLECTOR_READY", "FINAL_AUDIT", "COMMIT_PUBLICATION", "UPLOADER_READY":
			phase = refusal.phase
		default:
			phase = "UNKNOWN"
		}
	}
	return phase
}
