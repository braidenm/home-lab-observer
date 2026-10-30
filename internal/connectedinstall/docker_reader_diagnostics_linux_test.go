//go:build linux

package connectedinstall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

func TestDockerReaderFailurePhasesAreClosedAndSanitized(t *testing.T) {
	private := errors.New("hlc_private_provider_path_and_manager_output")
	cases := []struct {
		failure, phase string
		recovery       bool
		change         func(*dockerReaderOperations, context.CancelFunc)
	}{
		{"initial-inspect", "READER_STATE", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.inspect = func(context.Context, string) (workerInstance, error) { return workerInstance{}, private }
		}},
		{"pin", "READER_ACTIVATION_PIN", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.pin = func(uint32) (activationRecords, error) { return nil, private }
		}},
		{"clear", "READER_RESPONSE_CLEAR", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.clearResponse = func() error { return private }
		}},
		{"uncertain-start", "READER_SERVICE_START", true, nil},
		{"audit-before", "READER_AUDIT_BEFORE_REQUEST", false, nil},
		{"source-zero-device", "READER_AUDIT_BEFORE_REQUEST", false, nil},
		{"source-zero-inode", "READER_AUDIT_BEFORE_REQUEST", false, nil},
		{"entropy", "READER_REQUEST_BUILD", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.random = func([]byte) (int, error) { return 0, private }
		}},
		{"short-entropy", "READER_REQUEST_BUILD", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.random = func(b []byte) (int, error) { return len(b) - 1, nil }
		}},
		{"request.json", "READER_REQUEST_PUBLICATION", true, nil},
		{"response-error", "READER_RESPONSE_WAIT", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.response = func() (connectedactivation.Response, error) { return connectedactivation.Response{}, private }
		}},
		{"missing-response", "READER_RESPONSE_WAIT", false, nil},
		{"stale-response", "READER_RESPONSE_MATCH", false, nil},
		{"audit-response", "READER_AUDIT_AFTER_RESPONSE", false, nil},
		{"source-replaced", "READER_AUDIT_AFTER_RESPONSE", false, nil},
		{"source-device", "READER_AUDIT_AFTER_RESPONSE", false, nil},
		{"commit.json", "READER_COMMIT_PUBLICATION", true, nil},
		{"status-error", "READER_STATUS_WAIT", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.status = func() (connectedstatus.Record, error) { return connectedstatus.Record{}, private }
		}},
		{"malformed-status", "READER_STATUS_WAIT", false, func(o *dockerReaderOperations, _ context.CancelFunc) {
			o.status = func() (connectedstatus.Record, error) {
				return connectedstatus.Record{InvocationID: strings.Repeat("a", 32), State: private.Error()}, nil
			}
		}},
		{"stale-status", "READER_STATUS_WAIT", false, nil},
		{"changed-after-commit", "READER_STATUS_WAIT", false, nil},
		{"cancelled", "READER_COMMIT_PUBLICATION", true, nil},
		{"cancelled-status", "READER_FINAL_IDENTITY", false, func(o *dockerReaderOperations, cancel context.CancelFunc) {
			original := o.status
			o.status = func() (connectedstatus.Record, error) { value, err := original(); cancel(); return value, err }
		}},
		{"replaced", "READER_AUDIT_AFTER_RESPONSE", true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &readerFixture{t: t, fail: tc.failure, cancel: cancel}
			o := f.operations()
			if tc.change != nil {
				tc.change(&o, cancel)
			}
			_, err := startDockerReaderWith(ctx, dockerLifecycleConfig(t), o)
			if err == nil || LifecyclePhase(&lifecycleError{"READER_STARTUP", err}) != tc.phase || errors.Is(err, ErrRecovery) != tc.recovery {
				t.Fatal("reader failure lost its closed phase or recovery disposition")
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				if strings.Contains(cause.Error(), "private") || cause == private {
					t.Fatal("reader retained provider error details")
				}
			}
		})
	}
}

func TestDockerReaderCleanupCannotReplacePrimaryFailure(t *testing.T) {
	for _, tc := range []struct{ failure, phase string }{
		{"audit-before", "READER_AUDIT_BEFORE_REQUEST"}, {"uncertain-start", "READER_SERVICE_START"},
	} {
		f := &readerFixture{t: t, fail: tc.failure}
		o := f.operations()
		original := o.service
		o.service = func(ctx context.Context, verb, unit string) error {
			if verb == "stop" {
				return errors.New("hlc_private_cleanup_provider_text")
			}
			return original(ctx, verb, unit)
		}
		_, err := startDockerReaderWith(context.Background(), dockerLifecycleConfig(t), o)
		if LifecyclePhase(&lifecycleError{"READER_STARTUP", err}) != tc.phase || errors.Is(err, ErrUnsafe) != (tc.failure == "audit-before") || !errors.Is(err, ErrRecovery) || !f.active || f.committed {
			t.Fatal("cleanup hid primary refusal, failed to require recovery or released reader")
		}
		for cause := err; cause != nil; cause = errors.Unwrap(cause) {
			if strings.Contains(cause.Error(), "private") {
				t.Fatal("cleanup retained provider details")
			}
		}
	}
}

func TestDockerReaderInvalidRequestsRemainClosedBeforeOperations(t *testing.T) {
	c := dockerLifecycleConfig(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, cancelled} {
		_, err := startDockerReaderWith(ctx, c, dockerReaderOperations{})
		if LifecyclePhase(err) != "READER_REQUEST" || !errors.Is(err, ErrUnsafe) || errors.Is(err, ErrRecovery) {
			t.Fatal("invalid context lost its finite refusal")
		}
	}
	c.FrameGID = 0
	_, err := startDockerReaderWith(context.Background(), c, dockerReaderOperations{})
	if LifecyclePhase(err) != "READER_REQUEST" || !errors.Is(err, ErrUnsafe) {
		t.Fatal("invalid configuration reached reader operations")
	}
}
