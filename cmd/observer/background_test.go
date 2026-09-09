package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/background"
	"github.com/braidenm/home-lab-observer/internal/localauth"
)

type fakeBackgroundManager struct {
	status      background.Status
	settings    background.Settings
	action      string
	stopOptions background.StopOptions
	err         error
}

func (manager *fakeBackgroundManager) Enable(_ context.Context, settings background.Settings) (background.Status, error) {
	manager.action, manager.settings = "enable", settings
	return manager.status, manager.err
}
func (manager *fakeBackgroundManager) LoadSettings() (background.Settings, error) {
	manager.action = "load"
	return manager.settings, manager.err
}
func (manager *fakeBackgroundManager) Start(context.Context) (background.Status, error) {
	manager.action = "start"
	return manager.status, manager.err
}
func (manager *fakeBackgroundManager) Status(context.Context) (background.Status, error) {
	manager.action = "status"
	return manager.status, manager.err
}
func (manager *fakeBackgroundManager) Stop(_ context.Context, options background.StopOptions) (background.Status, error) {
	manager.action, manager.stopOptions = "stop", options
	return manager.status, manager.err
}
func (manager *fakeBackgroundManager) Restart(_ context.Context, options background.StopOptions) (background.Status, error) {
	manager.action, manager.stopOptions = "restart", options
	return manager.status, manager.err
}
func (manager *fakeBackgroundManager) Disable(_ context.Context, options background.StopOptions) (background.Status, error) {
	manager.action, manager.stopOptions = "disable", options
	return manager.status, manager.err
}

func TestBackgroundHelpDoesNotConstructManager(t *testing.T) {
	for _, args := range [][]string{{}, {"help"}, {"status", "--help"}} {
		var output, errorsOutput strings.Builder
		called := false
		code := runBackgroundWith(args, &output, &errorsOutput, func(background.Config) (background.Manager, error) {
			called = true
			return nil, errors.New("must not be called")
		}, func([]string, io.Writer, io.Writer) int { return 1 })
		if code != 0 || called {
			t.Fatalf("args=%v code=%d manager_called=%v", args, code, called)
		}
	}
}

func TestBackgroundStatusJSONIsStableAndContentFree(t *testing.T) {
	manager := &fakeBackgroundManager{status: background.Status{
		State: background.StateRunning, Code: background.CodeOK, Scope: background.ScopeUserSession,
		SessionLimitation: background.SessionLimitation, Readiness: background.ReadinessReady,
		Diagnostics: background.DiagnosticsAvailable, Registered: true, Running: true,
	}}
	var output, errorsOutput strings.Builder
	code := runBackgroundWith([]string{"status", "--install-root", filepath.Join(t.TempDir(), "managed"), "--json"}, &output, &errorsOutput, func(config background.Config) (background.Manager, error) {
		if config.InstallRoot == "" || config.Stopper == nil || config.Probe == nil {
			t.Fatal("background manager dependencies were incomplete")
		}
		return manager, nil
	}, func([]string, io.Writer, io.Writer) int { return 1 })
	if code != 0 || errorsOutput.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, errorsOutput.String())
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(output.String()), &status); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"code", "diagnostics", "readiness", "registered", "running", "schema_version", "scope", "session_limitation", "state"}
	actualKeys := make([]string, 0, len(status))
	for key := range status {
		actualKeys = append(actualKeys, key)
	}
	sort.Strings(actualKeys)
	if !reflect.DeepEqual(actualKeys, wantKeys) || status["diagnostics"] != "AVAILABLE" || strings.Contains(output.String(), "token") {
		t.Fatalf("unexpected status output: %s", output.String())
	}
}

func TestBackgroundEnableAndForceAreExplicit(t *testing.T) {
	manager := &fakeBackgroundManager{status: background.Status{State: background.StateRunning, Code: background.CodeOK, Scope: background.ScopeUserSession, SessionLimitation: background.SessionLimitation, Readiness: background.ReadinessReady, Diagnostics: background.DiagnosticsAvailable, Registered: true, Running: true}}
	factory := func(background.Config) (background.Manager, error) { return manager, nil }
	root, state := filepath.Join(t.TempDir(), "managed"), filepath.Join(t.TempDir(), "state")
	var output, errorsOutput strings.Builder
	if code := runBackgroundWith([]string{"enable", "--install-root", root, "--state-dir", state, "--listen", "127.0.0.1:19047"}, &output, &errorsOutput, factory, nil); code != 0 {
		t.Fatalf("enable code=%d stderr=%q", code, errorsOutput.String())
	}
	if manager.action != "enable" || manager.settings.StateDir != state || manager.settings.ListenAddress != "127.0.0.1:19047" || !strings.Contains(output.String(), "not before login") {
		t.Fatalf("enable did not preserve explicit settings and limitation: %+v %q", manager.settings, output.String())
	}
	output.Reset()
	if code := runBackgroundWith([]string{"stop", "--install-root", root, "--force"}, &output, &errorsOutput, factory, nil); code != 0 || !manager.stopOptions.Force {
		t.Fatalf("force stop code=%d options=%+v", code, manager.stopOptions)
	}
	if code := runBackgroundWith([]string{"status", "--install-root", root, "--force"}, io.Discard, io.Discard, factory, nil); code != 2 {
		t.Fatalf("status accepted force option: %d", code)
	}
}

func TestBackgroundRunLoadsClosedSettingsAndDiscardsManagerOutput(t *testing.T) {
	manager := &fakeBackgroundManager{settings: background.Settings{StateDir: filepath.Join(t.TempDir(), "state"), ListenAddress: "127.0.0.1:9847"}}
	var output, errorsOutput strings.Builder
	var got []string
	code := runBackgroundWith([]string{"run", "--install-root", filepath.Join(t.TempDir(), "managed")}, &output, &errorsOutput, func(background.Config) (background.Manager, error) { return manager, nil }, func(args []string, stdout, stderr io.Writer) int {
		got = append([]string(nil), args...)
		_, _ = io.WriteString(stdout, "synthetic-secret")
		_, _ = io.WriteString(stderr, "synthetic-secret")
		return 0
	})
	want := []string{"--internal-background-runtime", "--listen", "127.0.0.1:9847", "--state-dir", manager.settings.StateDir}
	if code != 0 || !reflect.DeepEqual(got, want) || output.Len() != 0 || errorsOutput.Len() != 0 {
		t.Fatalf("code=%d args=%v stdout=%q stderr=%q", code, got, output.String(), errorsOutput.String())
	}
}

func TestBackgroundErrorsExposeOnlyStableCode(t *testing.T) {
	manager := &fakeBackgroundManager{err: &background.Error{Code: background.CodeRegistrationMismatch, Err: errors.New("synthetic-secret-path")}}
	var output, errorsOutput strings.Builder
	code := runBackgroundWith([]string{"status", "--install-root", filepath.Join(t.TempDir(), "managed")}, &output, &errorsOutput, func(background.Config) (background.Manager, error) { return manager, nil }, nil)
	if code != 1 || !strings.Contains(errorsOutput.String(), string(background.CodeRegistrationMismatch)) || strings.Contains(errorsOutput.String(), "synthetic-secret") {
		t.Fatalf("code=%d stderr=%q", code, errorsOutput.String())
	}
}

func TestLocalProbeSeparatesReadinessAndAuthenticatedDiagnostics(t *testing.T) {
	state := filepath.Join(canonicalTestTempDir(t), "state")
	token, _, err := localauth.Ensure(filepath.Join(state, "local-api.token"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/health/ready":
			_, _ = io.WriteString(writer, `{"status":"UP"}`)
		case "/api/v1/diagnostics/health":
			if request.Header.Get("Authorization") != "Bearer "+token {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(writer, `{"schema_version":"observer-diagnostics-health/v1","generated_at":"2026-09-09T19:00:00Z","enabled":true,"available":true,"state":"AVAILABLE","reason_code":null,"limits":{"max_files":5,"max_file_bytes":2097152,"max_total_bytes":10485760,"max_record_bytes":8192,"max_age_seconds":604800},"usage":{"total_bytes":4096,"file_count":1},"counters":{"dropped_records":0,"write_failures":0},"policy":{"data_classification":"PUBLIC_METADATA","contains_log_contents":false,"contains_paths":false,"remote_upload_eligible":false},"future_metadata":"discarded"}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	settings := background.Settings{StateDir: state, ListenAddress: strings.TrimPrefix(server.URL, "http://")}
	got := (localReadinessProbe{}).Probe(context.Background(), settings)
	if got.Readiness != background.ReadinessReady || got.Diagnostics != background.DiagnosticsAvailable {
		t.Fatalf("probe=%+v", got)
	}

	missingState := filepath.Join(canonicalTestTempDir(t), "missing-state")
	got = (localReadinessProbe{}).Probe(context.Background(), background.Settings{StateDir: missingState, ListenAddress: settings.ListenAddress})
	if got.Readiness != background.ReadinessReady || got.Diagnostics != background.DiagnosticsUnknown {
		t.Fatalf("missing-token probe=%+v", got)
	}
	if _, err := os.Lstat(filepath.Join(missingState, "local-api.token")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status probe created a missing token: %v", err)
	}
}

func canonicalTestTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
