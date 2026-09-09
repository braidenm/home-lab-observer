package background

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeAdapter struct {
	mu          sync.Mutex
	state       managerState
	registered  int
	started     int
	stopped     []bool
	removed     int
	stayRunning bool
}

func (f *fakeAdapter) registration(Settings) (registration, error) {
	return registration{fileName: "task.xml", content: []byte("fixed-registration\n")}, nil
}
func (f *fakeAdapter) available(context.Context) bool { return f.state.available }
func (f *fakeAdapter) inspect(context.Context, registration) (managerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}
func (f *fakeAdapter) register(context.Context, string, registration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered++
	f.state.registered = true
	return nil
}
func (f *fakeAdapter) start(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	f.state.running = true
	return nil
}
func (f *fakeAdapter) stop(_ context.Context, force bool, stopper GracefulStopper, settings Settings) error {
	f.mu.Lock()
	f.stopped = append(f.stopped, force)
	f.mu.Unlock()
	if !force && stopper != nil {
		if err := stopper.RequestStop(context.Background(), settings.StateDir); err != nil {
			return err
		}
	}
	f.mu.Lock()
	if !f.stayRunning {
		f.state.running = false
	}
	f.mu.Unlock()
	return nil
}
func (f *fakeAdapter) unregister(context.Context, string, registration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed++
	f.state.registered = false
	return nil
}

type fakeStopper struct{ calls int }

func (f *fakeStopper) RequestStop(context.Context, string) error { f.calls++; return nil }

type fakeProbe struct{ value Readiness }

func (f fakeProbe) Probe(context.Context, Settings) ProbeStatus {
	return ProbeStatus{Readiness: f.value, Diagnostics: DiagnosticsAvailable}
}

func testController(t *testing.T) (*controller, *fakeAdapter, *fakeStopper, Settings) {
	t.Helper()
	root := filepath.Join(canonicalTempDir(t), "programs")
	stateDir := filepath.Join(canonicalTempDir(t), "state")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	adapter := &fakeAdapter{state: managerState{available: true}}
	stopper := &fakeStopper{}
	return newController(root, adapter, stopper, fakeProbe{value: ReadinessReady}), adapter, stopper, Settings{StateDir: stateDir, ListenAddress: "127.0.0.1:9847"}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestLifecycleIsIdempotentAndGracefulByDefault(t *testing.T) {
	controller, adapter, stopper, settings := testController(t)
	ctx := context.Background()
	status, err := controller.Enable(ctx, settings)
	if err != nil || status.State != StateRunning || status.Readiness != ReadinessReady || status.Diagnostics != DiagnosticsAvailable {
		t.Fatalf("enable status = %#v, %v", status, err)
	}
	if _, err := controller.Enable(ctx, settings); err != nil {
		t.Fatalf("idempotent enable: %v", err)
	}
	if adapter.registered != 1 || adapter.started != 1 {
		t.Fatalf("register/start calls = %d/%d", adapter.registered, adapter.started)
	}
	status, err = controller.Stop(ctx, StopOptions{})
	if err != nil || status.State != StateRegisteredStopped || stopper.calls != 1 {
		t.Fatalf("graceful stop = %#v, %v, calls %d", status, err, stopper.calls)
	}
	status, err = controller.Restart(ctx, StopOptions{Force: true})
	if err != nil || status.State != StateRunning || adapter.stopped[len(adapter.stopped)-1] {
		// Restarting an already stopped job does not terminate anything.
		t.Fatalf("restart = %#v, %v, stops %#v", status, err, adapter.stopped)
	}
	status, err = controller.Disable(ctx, StopOptions{})
	if err != nil || status.State != StateNotRegistered || adapter.removed != 1 {
		t.Fatalf("disable = %#v, %v", status, err)
	}
	if _, err := os.Stat(filepath.Join(controller.backgroundDir, markerFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disable retained managed marker")
	}
	status, err = controller.Disable(ctx, StopOptions{})
	if err != nil || status.State != StateNotRegistered {
		t.Fatalf("idempotent disable = %#v, %v", status, err)
	}
}

func TestExplicitForceNeverFollowsGracefulFailure(t *testing.T) {
	controller, adapter, _, settings := testController(t)
	ctx := context.Background()
	if _, err := controller.Enable(ctx, settings); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.state.running = true
	adapter.mu.Unlock()
	if _, err := controller.Stop(ctx, StopOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if len(adapter.stopped) != 1 || !adapter.stopped[0] {
		t.Fatalf("force flags = %#v", adapter.stopped)
	}
}

func TestStopWaitsForConfirmedTerminalState(t *testing.T) {
	controller, adapter, _, settings := testController(t)
	controller.gracefulWait = 150 * time.Millisecond
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.stayRunning = true
	adapter.mu.Unlock()
	if _, err := controller.Restart(context.Background(), StopOptions{}); !IsCode(err, CodeGracefulStopFailed) {
		t.Fatalf("restart timeout error = %v", err)
	}
	if adapter.started != 1 {
		t.Fatal("restart started a replacement before the old instance stopped")
	}
	if adapter.removed != 0 {
		t.Fatal("stop timeout removed registration")
	}
}

func TestRestartWaitsForDelayedTerminalState(t *testing.T) {
	controller, adapter, _, settings := testController(t)
	controller.gracefulWait = time.Second
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.stayRunning = true
	adapter.mu.Unlock()
	go func() {
		time.Sleep(60 * time.Millisecond)
		adapter.mu.Lock()
		adapter.state.running = false
		adapter.mu.Unlock()
	}()
	started := time.Now()
	if _, err := controller.Restart(context.Background(), StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 50*time.Millisecond || adapter.started != 2 {
		t.Fatal("restart did not wait for the prior instance to stop")
	}
}

func TestDisableRefusesUnavailableManagerAndPreservesEvidence(t *testing.T) {
	controller, adapter, _, settings := testController(t)
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.state.available = false
	adapter.mu.Unlock()
	if _, err := controller.Disable(context.Background(), StopOptions{}); !IsCode(err, CodeManagerUnavailable) {
		t.Fatalf("disable error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(controller.backgroundDir, markerFile)); err != nil {
		t.Fatal("unavailable disable removed managed evidence")
	}
}

func TestUnknownAndChangedManagedStateIsRefused(t *testing.T) {
	controller, _, _, settings := testController(t)
	if err := os.MkdirAll(controller.backgroundDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controller.backgroundDir, "unknown"), []byte("owner data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeUnsafeManagedState) {
		t.Fatalf("unknown state error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(controller.backgroundDir, "unknown")); err != nil {
		t.Fatal("unknown data was removed")
	}
}

func TestInterruptedEnableKnownFilesAreRecovered(t *testing.T) {
	controller, _, _, settings := testController(t)
	if err := os.MkdirAll(controller.backgroundDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controller.backgroundDir, settingsFile), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controller.backgroundDir, "task.xml"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatalf("recover enable: %v", err)
	}
	if _, err := controller.LoadSettings(); err != nil {
		t.Fatalf("load recovered settings: %v", err)
	}
}

func TestInterruptedAtomicPromotionsAreRecovered(t *testing.T) {
	for _, name := range []string{settingsFile + ".next", "task.xml.next", markerFile + ".next"} {
		t.Run(name, func(t *testing.T) {
			controller, _, _, settings := testController(t)
			if err := os.MkdirAll(controller.backgroundDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(controller.backgroundDir, name), []byte("partial"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := controller.Enable(context.Background(), settings); err != nil {
				t.Fatalf("recover %s: %v", name, err)
			}
		})
	}
}

func TestInterruptedDisableCleanupIsRecoverable(t *testing.T) {
	controller, adapter, _, settings := testController(t)
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	adapter.mu.Lock()
	adapter.state.running = false
	adapter.state.registered = false
	adapter.mu.Unlock()
	if err := writePrivateAtomic(filepath.Join(controller.backgroundDir, disablingFile), []byte(backgroundMarker+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(controller.backgroundDir, markerFile)); err != nil {
		t.Fatal(err)
	}
	status, err := controller.Disable(context.Background(), StopOptions{})
	if err != nil || status.State != StateNotRegistered {
		t.Fatalf("resume disable = %#v, %v", status, err)
	}
	if _, err := os.Stat(filepath.Join(controller.backgroundDir, disablingFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted disable marker remains")
	}
}

func TestLinkedBackgroundAreaIsRejectedBeforeMutation(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(canonicalTempDir(t), "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "background")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	controller := newController(root, &fakeAdapter{}, nil, nil)
	if _, err := controller.lock(true); err == nil {
		t.Fatal("linked background area accepted")
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatal("linked target permissions were mutated")
	}
}

func TestUnknownRealBackgroundAreaPermissionsAreNotMutated(t *testing.T) {
	controller, _, _, settings := testController(t)
	if err := os.Mkdir(controller.backgroundDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(controller.backgroundDir, "owner-file"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(controller.backgroundDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeUnsafeManagedState) {
		t.Fatalf("error = %v", err)
	}
	after, err := os.Stat(controller.backgroundDir)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatal("unknown directory permissions were mutated")
	}
	if _, err := os.Stat(filepath.Join(controller.backgroundDir, "owner-file")); err != nil {
		t.Fatal("unknown file was removed")
	}
}

func TestOperationsAreSerializedByOSLock(t *testing.T) {
	controller, _, _, _ := testController(t)
	unlock, err := controller.lock(true)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := controller.Start(context.Background()); !IsCode(err, CodeOperationActive) {
		t.Fatalf("concurrent operation error = %v", err)
	}
}

func TestInstallAndBackgroundOperationsShareFailClosedGuard(t *testing.T) {
	controller, _, _, settings := testController(t)
	guardPath := filepath.Join(controller.installRoot, installGuard)
	if err := os.Mkdir(guardPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeOperationActive) {
		t.Fatalf("Enable error = %v, want operation active", err)
	}
	if _, err := os.Lstat(controller.backgroundDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("contended background operation mutated state: %v", err)
	}
	if err := os.Remove(guardPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(guardPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guardPath, guardOwner), []byte(guardOwnerMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatalf("retry did not recover crashed background guard: %v", err)
	}
	if _, err := os.Lstat(guardPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful operation retained shared guard: %v", err)
	}
	releaseBackground, err := controller.lock(false)
	if err != nil {
		t.Fatal(err)
	}
	contender := newController(controller.installRoot, &fakeAdapter{state: managerState{available: true}}, &fakeStopper{}, fakeProbe{value: ReadinessReady})
	if _, err := contender.Enable(context.Background(), settings); !IsCode(err, CodeOperationActive) {
		releaseBackground()
		t.Fatalf("concurrent background operation = %v, want operation active", err)
	}
	if err := os.Mkdir(guardPath, 0o700); !errors.Is(err, os.ErrExist) {
		releaseBackground()
		t.Fatalf("installer guard acquisition during background operation = %v, want already exists", err)
	}
	releaseBackground()
	if _, err := os.Lstat(guardPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("background operation release retained shared guard: %v", err)
	}
}

func TestSettingsAreClosedAndOutsideProgramFiles(t *testing.T) {
	controller, _, _, settings := testController(t)
	settings.StateDir = filepath.Join(controller.installRoot, "state")
	if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeInvalidSettings) {
		t.Fatalf("overlapping state error = %v", err)
	}
	settings.StateDir = filepath.Join(canonicalTempDir(t), "state")
	if _, err := controller.Enable(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(controller.backgroundDir, settingsFile)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents[len(contents)-2] = ','
	contents = append(contents[:len(contents)-1], []byte(`"extra":true}`+"\n")...)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.LoadSettings(); err == nil {
		t.Fatal("unknown settings field was accepted")
	}
}
