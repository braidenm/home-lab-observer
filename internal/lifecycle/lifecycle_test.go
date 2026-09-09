package lifecycle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNonceBoundRequestStopsAndWaitsForCleanClose(t *testing.T) {
	state := testStateDir(t)
	endpoint, err := New(Config{
		StateDir: state, Now: func() time.Time { return time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC) },
		Random: strings.NewReader(strings.Repeat("a", nonceBytes)), PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := readInstance(filepath.Join(state, controlDirectory, instanceName))
	if err != nil {
		t.Fatal(err)
	}
	if instance.Nonce != base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", nonceBytes))) || instance.StartedAt != "2026-09-09T18:00:00Z" {
		t.Fatalf("instance = %+v", instance)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- RequestStop(ctx, state) }()
	select {
	case <-endpoint.StopRequested():
	case <-ctx.Done():
		t.Fatal("stop request was not delivered")
	}
	select {
	case err := <-result:
		t.Fatalf("request returned before clean close: %v", err)
	default:
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(state, controlDirectory, instanceName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("instance publication remains: %v", err)
	}
}

func TestExclusiveInstanceAndIdempotentClose(t *testing.T) {
	state := testStateDir(t)
	endpoint, err := New(Config{StateDir: state, PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{StateDir: state}); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second New error = %v", err)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAbandonPreservesFailureEvidenceAndAllowsValidatedRecovery(t *testing.T) {
	state := testStateDir(t)
	first, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("a", nonceBytes)), PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, controlDirectory, instanceName)
	old, err := readInstance(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Abandon(); err != nil {
		t.Fatal(err)
	}
	if err := first.Abandon(); err != nil {
		t.Fatal(err)
	}
	retained, err := readInstance(path)
	if err != nil || retained.Nonce != old.Nonce {
		t.Fatalf("abandoned instance was not preserved: %+v, %v", retained, err)
	}

	second, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("b", nonceBytes))})
	if err != nil {
		t.Fatal(err)
	}
	current, err := readInstance(path)
	if err != nil || current.Nonce == old.Nonce {
		t.Fatalf("stale instance was not replaced: %+v, %v", current, err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAbandonWinsRaceWithCloseWithoutDeletingPublication(t *testing.T) {
	state := testStateDir(t)
	endpoint, err := New(Config{StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Abandon(); err != nil {
		t.Fatal(err)
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstance(filepath.Join(state, controlDirectory, instanceName)); err != nil {
		t.Fatalf("Close after Abandon changed terminal outcome: %v", err)
	}
}

func TestAbandonCannotAcknowledgeWaitingStopRequester(t *testing.T) {
	state := testStateDir(t)
	endpoint, err := New(Config{StateDir: state, PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- RequestStop(ctx, state) }()
	requestPath := filepath.Join(state, controlDirectory, requestName)
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		if _, err := os.Lstat(requestPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stop request was not published")
		}
		time.Sleep(time.Millisecond)
	}
	if err := endpoint.Abandon(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("abandoned endpoint falsely acknowledged stop: %v", err)
	}
	if _, err := readInstance(filepath.Join(state, controlDirectory, instanceName)); err != nil {
		t.Fatalf("abandoned instance evidence missing: %v", err)
	}
}

func TestMalformedOversizedAndWrongInstanceRequestsNeverSignal(t *testing.T) {
	tests := []struct {
		name     string
		contents []byte
	}{
		{"malformed", []byte(`{"schema_version":"observer-lifecycle-stop/v1","nonce":`)},
		{"unknown-field", []byte(`{"schema_version":"observer-lifecycle-stop/v1","nonce":"` + nonce('x') + `","command":"delete"}`)},
		{"wrong-instance", requestJSON(t, nonce('x'))},
		{"oversized", []byte(strings.Repeat("x", int(maxControlBytes+1)))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := testStateDir(t)
			endpoint, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("a", nonceBytes)), PollInterval: 10 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state, controlDirectory, requestName)
			if err := os.WriteFile(path, test.contents, 0o600); err != nil {
				t.Fatal(err)
			}
			select {
			case <-endpoint.StopRequested():
				t.Fatal("unsafe request signaled stop")
			case <-time.After(50 * time.Millisecond):
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := endpoint.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOldNonceReplayDoesNotStopNewInstance(t *testing.T) {
	state := testStateDir(t)
	first, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("a", nonceBytes)), PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	old, err := readInstance(filepath.Join(state, controlDirectory, instanceName))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("b", nonceBytes)), PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, controlDirectory, requestName)
	if err := os.WriteFile(path, requestJSON(t, old.Nonce), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second.StopRequested():
		t.Fatal("replayed nonce signaled new instance")
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestStopReturnsTypedStateErrors(t *testing.T) {
	state := testStateDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := RequestStop(ctx, state); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("not-running error = %v", err)
	}

	endpoint, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("a", nonceBytes)), PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	path := filepath.Join(state, controlDirectory, requestName)
	if err := os.WriteFile(path, requestJSON(t, nonce('x')), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequestStop(ctx, state); !errors.Is(err, ErrWrongInstance) {
		t.Fatalf("wrong-instance error = %v", err)
	}
}

func TestCancelledRequestDoesNotCreateControlState(t *testing.T) {
	state := testStateDir(t)
	directory := filepath.Join(state, controlDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RequestStop(ctx, state); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled request created files: %v", entries)
	}
}

func TestMalformedExistingInstanceIsPreservedAndRefused(t *testing.T) {
	state := testStateDir(t)
	directory := filepath.Join(state, controlDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, instanceName)
	contents := []byte(`{"schema_version":"observer-lifecycle-instance/v1","nonce":"not-a-nonce","started_at":"2026-09-09T18:00:00Z"}`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{StateDir: state}); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("malformed state was changed: %q, %v", got, err)
	}
}

func TestRequestTimeoutDoesNotForceAndLateRequestRemainsValid(t *testing.T) {
	state := testStateDir(t)
	endpoint, err := New(Config{StateDir: state, PollInterval: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := RequestStop(ctx, state); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	select {
	case <-endpoint.StopRequested():
	case <-time.After(2 * time.Second):
		t.Fatal("timed-out request was not eventually handled gracefully")
	}
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewRecoversValidatedStaleFiles(t *testing.T) {
	state := testStateDir(t)
	directory := filepath.Join(state, controlDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := instanceFile{SchemaVersion: "observer-lifecycle-instance/v1", Nonce: nonce('a'), StartedAt: "2026-09-09T18:00:00Z"}
	contents, _ := json.Marshal(stale)
	if err := os.WriteFile(filepath.Join(directory, instanceName), append(contents, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, requestName), requestJSON(t, stale.Nonce), 0o600); err != nil {
		t.Fatal(err)
	}
	endpoint, err := New(Config{StateDir: state, Random: strings.NewReader(strings.Repeat("b", nonceBytes))})
	if err != nil {
		t.Fatal(err)
	}
	defer endpoint.Close()
	current, err := readInstance(filepath.Join(directory, instanceName))
	if err != nil {
		t.Fatal(err)
	}
	if current.Nonce == stale.Nonce {
		t.Fatal("stale nonce was reused")
	}
	if _, err := os.Lstat(filepath.Join(directory, requestName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale request remains: %v", err)
	}
}

func TestUnsafeAncestorAndHardLinkedControlFilesAreRejected(t *testing.T) {
	t.Run("linked ancestor", func(t *testing.T) {
		root := canonicalTempDir(t)
		target := filepath.Join(root, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "linked")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := New(Config{StateDir: filepath.Join(link, "state")}); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("error = %v", err)
		}
		if _, err := os.Lstat(filepath.Join(target, "state")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("linked target mutated: %v", err)
		}
	})
	t.Run("hard linked instance", func(t *testing.T) {
		state := testStateDir(t)
		directory := filepath.Join(state, controlDirectory)
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(state, "outside")
		stale := instanceFile{SchemaVersion: "observer-lifecycle-instance/v1", Nonce: nonce('a'), StartedAt: "2026-09-09T18:00:00Z"}
		contents, _ := json.Marshal(stale)
		if err := os.WriteFile(outside, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(outside, filepath.Join(directory, instanceName)); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		if _, err := New(Config{StateDir: state}); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("error = %v", err)
		}
		got, err := os.ReadFile(outside)
		if err != nil || string(got) != string(contents) {
			t.Fatalf("outside changed: %q, %v", got, err)
		}
	})
}

func requestJSON(t *testing.T, value string) []byte {
	t.Helper()
	contents, err := json.Marshal(requestFile{SchemaVersion: "observer-lifecycle-stop/v1", Nonce: value})
	if err != nil {
		t.Fatal(err)
	}
	return append(contents, '\n')
}

func nonce(value byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string(value), nonceBytes)))
}

func testStateDir(t *testing.T) string {
	t.Helper()
	state := filepath.Join(canonicalTempDir(t), "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	return state
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
