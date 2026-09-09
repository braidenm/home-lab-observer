package background

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

const (
	markerFile    = ".managed"
	settingsFile  = "settings.json"
	lockDirectory = ".operation-lock"
	disablingFile = ".disabling"
)

type registration struct {
	fileName string
	content  []byte
}

type managerState struct {
	available  bool
	registered bool
	running    bool
}

type platformAdapter interface {
	registration(Settings) (registration, error)
	available(context.Context) bool
	inspect(context.Context, registration) (managerState, error)
	register(context.Context, string, registration) error
	start(context.Context) error
	stop(context.Context, bool, GracefulStopper, Settings) error
	unregister(context.Context, string, registration) error
}

type controller struct {
	installRoot   string
	backgroundDir string
	adapter       platformAdapter
	stopper       GracefulStopper
	probe         ReadinessProbe
	gracefulWait  time.Duration
}

func NewManager(config Config) (Manager, error) {
	root, err := validateInstallRoot(config.InstallRoot)
	if err != nil {
		return nil, err
	}
	adapter, err := newPlatformAdapter(execRunner{timeout: config.Timeout}, root)
	if err != nil {
		return nil, err
	}
	return newController(root, adapter, config.Stopper, config.Probe), nil
}

func newController(root string, adapter platformAdapter, stopper GracefulStopper, probe ReadinessProbe) *controller {
	return &controller{installRoot: root, backgroundDir: filepath.Join(root, "background"), adapter: adapter, stopper: stopper, probe: probe, gracefulWait: GracefulWait}
}

func (c *controller) Enable(ctx context.Context, requested Settings) (Status, error) {
	settings, err := validateSettings(requested)
	if err != nil {
		return Status{}, err
	}
	if pathWithin(c.installRoot, settings.StateDir) || pathWithin(settings.StateDir, c.installRoot) {
		return Status{}, coded(CodeInvalidSettings, errors.New("state directory must be separate from the program install root"))
	}
	unlock, err := c.lock(true)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	if _, err := os.Lstat(filepath.Join(c.backgroundDir, disablingFile)); err == nil {
		return Status{}, coded(CodeOperationActive, errors.New("background disable is incomplete; run disable again"))
	}
	registration, err := c.adapter.registration(settings)
	if err != nil {
		return Status{}, coded(CodeInvalidSettings, err)
	}
	managed, err := c.hasManagedState()
	if err != nil {
		if _, markerErr := os.Lstat(filepath.Join(c.backgroundDir, markerFile)); errors.Is(markerErr, os.ErrNotExist) {
			if recoveryErr := c.recoverPartialEnable(registration); recoveryErr != nil {
				return Status{}, recoveryErr
			}
			managed = false
		} else {
			return Status{}, err
		}
	}
	if managed {
		stored, err := readSettings(filepath.Join(c.backgroundDir, settingsFile))
		if err != nil || !reflect.DeepEqual(stored, settings) {
			return Status{}, coded(CodeRegistrationMismatch, errors.New("existing settings differ; disable before reconfiguring"))
		}
		if err := c.validateManagedFiles(registration); err != nil {
			return Status{}, err
		}
	} else {
		if err := c.recoverPartialEnable(registration); err != nil {
			return Status{}, err
		}
		if err := c.initializeManagedState(settings, registration); err != nil {
			return Status{}, err
		}
	}
	state, err := c.adapter.inspect(ctx, registration)
	if err != nil {
		return Status{}, err
	}
	if !state.available {
		return statusFor(state, ReadinessUnknown), coded(CodeManagerUnavailable, errors.New("user-session manager is unavailable"))
	}
	if !state.registered {
		if err := c.adapter.register(ctx, filepath.Join(c.backgroundDir, registration.fileName), registration); err != nil {
			return Status{}, coded(CodeManagerOperationFailed, err)
		}
	}
	state, err = c.adapter.inspect(ctx, registration)
	if err != nil {
		return Status{}, err
	}
	if !state.running {
		if err := c.adapter.start(ctx); err != nil {
			return Status{}, coded(CodeManagerOperationFailed, err)
		}
	}
	return c.statusLocked(ctx, settings, registration)
}

// LoadSettings validates and returns the closed persisted run configuration.
// It never registers, starts, stops, or otherwise changes the background job.
func (c *controller) LoadSettings() (Settings, error) {
	settings, _, err := c.loadManaged()
	return settings, err
}

func (c *controller) Start(ctx context.Context) (Status, error) {
	return c.withManaged(ctx, func(settings Settings, registration registration) (Status, error) {
		if c.disabling() {
			return Status{}, coded(CodeOperationActive, errors.New("background disable is incomplete"))
		}
		state, err := c.adapter.inspect(ctx, registration)
		if err != nil {
			return Status{}, err
		}
		if !state.available {
			return statusFor(state, ReadinessUnknown), coded(CodeManagerUnavailable, errors.New("user-session manager is unavailable"))
		}
		if !state.registered {
			return statusFor(state, ReadinessUnknown), coded(CodeRegistrationMismatch, errors.New("managed registration is missing"))
		}
		if !state.running {
			if err := c.adapter.start(ctx); err != nil {
				return Status{}, coded(CodeManagerOperationFailed, err)
			}
		}
		return c.statusLocked(ctx, settings, registration)
	})
}

func (c *controller) Status(ctx context.Context) (Status, error) {
	managed, err := c.hasManagedState()
	if err != nil {
		return Status{}, err
	}
	if !managed {
		return statusFor(managerState{available: c.adapter.available(ctx)}, ReadinessUnknown), nil
	}
	settings, registration, err := c.loadManaged()
	if err != nil {
		return Status{}, err
	}
	return c.statusLocked(ctx, settings, registration)
}

func (c *controller) Stop(ctx context.Context, options StopOptions) (Status, error) {
	return c.stop(ctx, options, false)
}

func (c *controller) Restart(ctx context.Context, options StopOptions) (Status, error) {
	return c.stop(ctx, options, true)
}

func (c *controller) stop(ctx context.Context, options StopOptions, restart bool) (Status, error) {
	return c.withManaged(ctx, func(settings Settings, registration registration) (Status, error) {
		if c.disabling() {
			return Status{}, coded(CodeOperationActive, errors.New("background disable is incomplete"))
		}
		state, err := c.adapter.inspect(ctx, registration)
		if err != nil {
			return Status{}, err
		}
		if !state.available {
			return statusFor(state, ReadinessUnknown), coded(CodeManagerUnavailable, errors.New("user-session manager is unavailable"))
		}
		if !state.registered {
			return statusFor(state, ReadinessUnknown), coded(CodeRegistrationMismatch, errors.New("managed registration is missing"))
		}
		if state.running {
			stopCtx, cancel := context.WithTimeout(ctx, c.gracefulWait)
			err = c.adapter.stop(stopCtx, options.Force, c.stopper, settings)
			if err == nil {
				err = c.waitForStopped(stopCtx, registration)
			}
			cancel()
			if err != nil {
				code := CodeGracefulStopFailed
				if options.Force {
					code = CodeManagerOperationFailed
				}
				return Status{}, coded(code, err)
			}
		}
		if restart {
			if err := c.adapter.start(ctx); err != nil {
				return Status{}, coded(CodeManagerOperationFailed, err)
			}
		}
		return c.statusLocked(ctx, settings, registration)
	})
}

func (c *controller) Disable(ctx context.Context, options StopOptions) (Status, error) {
	if status, recovered, err := c.resumeInterruptedDisable(ctx); recovered || err != nil {
		return status, err
	}
	return c.withManaged(ctx, func(settings Settings, registration registration) (Status, error) {
		state, err := c.adapter.inspect(ctx, registration)
		if err != nil {
			return Status{}, err
		}
		if !state.available {
			return statusFor(state, ReadinessUnknown), coded(CodeManagerUnavailable, errors.New("refusing disable while the user-session manager is unavailable"))
		}
		if state.running {
			stopCtx, cancel := context.WithTimeout(ctx, c.gracefulWait)
			err = c.adapter.stop(stopCtx, options.Force, c.stopper, settings)
			if err == nil {
				err = c.waitForStopped(stopCtx, registration)
			}
			cancel()
			if err != nil {
				code := CodeGracefulStopFailed
				if options.Force {
					code = CodeManagerOperationFailed
				}
				return Status{}, coded(code, err)
			}
		}
		if state.registered {
			if err := writePrivateAtomic(filepath.Join(c.backgroundDir, disablingFile), []byte(backgroundMarker+"\n")); err != nil {
				return Status{}, coded(CodeUnsafeManagedState, err)
			}
			if err := c.adapter.unregister(ctx, filepath.Join(c.backgroundDir, registration.fileName), registration); err != nil {
				return Status{}, coded(CodeManagerOperationFailed, err)
			}
		}
		if err := c.removeManagedState(registration); err != nil {
			return Status{}, err
		}
		return statusFor(managerState{available: state.available}, ReadinessUnknown), nil
	})
}

func (c *controller) disabling() bool {
	_, err := os.Lstat(filepath.Join(c.backgroundDir, disablingFile))
	return err == nil
}

func (c *controller) resumeInterruptedDisable(ctx context.Context) (Status, bool, error) {
	if _, err := os.Lstat(filepath.Join(c.backgroundDir, markerFile)); err == nil {
		return Status{}, false, nil
	}
	transition, err := readSmallRegular(filepath.Join(c.backgroundDir, disablingFile), 128)
	if errors.Is(err, os.ErrNotExist) {
		return Status{}, false, nil
	}
	if err != nil || string(transition) != backgroundMarker+"\n" {
		return Status{}, true, coded(CodeUnsafeManagedState, errors.New("interrupted disable marker is unsafe"))
	}
	unlock, err := c.lock(false)
	if err != nil {
		return Status{}, true, err
	}
	defer unlock()
	allowed := map[string]bool{lockDirectory: true, disablingFile: true, settingsFile: true, "home-lab-observer.service": true, "com.braidenm.home-lab-observer.plist": true, "task.xml": true}
	entries, err := os.ReadDir(c.backgroundDir)
	if err != nil {
		return Status{}, true, coded(CodeUnsafeManagedState, err)
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return Status{}, true, coded(CodeUnsafeManagedState, fmt.Errorf("unknown interrupted-disable entry: %s", entry.Name()))
		}
		if entry.Name() != lockDirectory {
			if _, err := readSmallRegular(filepath.Join(c.backgroundDir, entry.Name()), 64<<10); err != nil {
				return Status{}, true, coded(CodeUnsafeManagedState, err)
			}
		}
	}
	for _, name := range []string{settingsFile, "home-lab-observer.service", "com.braidenm.home-lab-observer.plist", "task.xml", disablingFile} {
		_ = os.Remove(filepath.Join(c.backgroundDir, name))
	}
	return statusFor(managerState{available: c.adapter.available(ctx)}, ReadinessUnknown), true, nil
}

func (c *controller) waitForStopped(ctx context.Context, registration registration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := c.adapter.inspect(ctx, registration)
		if err != nil {
			return err
		}
		if !state.available {
			return errors.New("user-session manager became unavailable while waiting for stop")
		}
		if !state.registered || !state.running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *controller) withManaged(ctx context.Context, operation func(Settings, registration) (Status, error)) (Status, error) {
	unlock, err := c.lock(false)
	if err != nil {
		return Status{}, err
	}
	defer unlock()
	settings, registration, err := c.loadManaged()
	if err != nil {
		return Status{}, err
	}
	return operation(settings, registration)
}

func (c *controller) statusLocked(ctx context.Context, settings Settings, registration registration) (Status, error) {
	state, err := c.adapter.inspect(ctx, registration)
	if err != nil {
		return Status{}, coded(CodeManagerOperationFailed, err)
	}
	readiness := ReadinessUnknown
	diagnostics := DiagnosticsUnknown
	if state.running && c.probe != nil {
		probe := c.probe.Probe(ctx, settings)
		readiness, diagnostics = probe.Readiness, probe.Diagnostics
	}
	status := statusFor(state, readiness)
	status.Diagnostics = diagnostics
	return status, nil
}

func statusFor(manager managerState, readiness Readiness) Status {
	status := Status{Scope: ScopeUserSession, SessionLimitation: SessionLimitation, Readiness: readiness, Diagnostics: DiagnosticsUnknown, Registered: manager.registered, Running: manager.running}
	switch {
	case !manager.available:
		status.State, status.Code = StateManagerUnavailable, CodeManagerUnavailable
	case !manager.registered:
		status.State, status.Code = StateNotRegistered, CodeNotRegistered
	case !manager.running:
		status.State, status.Code = StateRegisteredStopped, CodeStopped
	case readiness == ReadinessUnreachable:
		status.State, status.Code = StateUnreachable, CodeUnreachable
	case readiness == ReadinessNotReady:
		status.State, status.Code = StateNotReady, CodeNotReady
	case readiness == ReadinessUnknown:
		status.State, status.Code = StateRunning, CodeReadinessUnknown
	default:
		status.State, status.Code = StateRunning, CodeOK
	}
	return status
}

func (c *controller) lock(create bool) (func(), error) {
	createdDirectory := false
	if create {
		if err := os.Mkdir(c.backgroundDir, 0o700); err == nil {
			createdDirectory = true
		} else if !errors.Is(err, os.ErrExist) {
			return nil, coded(CodeUnsafeManagedState, err)
		}
	}
	info, err := os.Lstat(c.backgroundDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || rejectReparse(c.backgroundDir) != nil {
		return nil, coded(CodeNotRegistered, errors.New("background profile is not registered"))
	}
	if createdDirectory {
		if err := ownerfs.RestrictDirectory(c.backgroundDir); err != nil {
			return nil, coded(CodeUnsafeManagedState, err)
		}
	}
	lock := filepath.Join(c.backgroundDir, lockDirectory)
	file, err := os.OpenFile(lock, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	created := err == nil
	if errors.Is(err, os.ErrExist) {
		pathInfo, validateErr := ownerfs.ValidateRegular(lock, 64)
		if validateErr != nil {
			return nil, coded(CodeUnsafeManagedState, validateErr)
		}
		file, err = os.OpenFile(lock, os.O_RDWR, 0o600)
		if err == nil {
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(opened, pathInfo) {
				file.Close()
				return nil, coded(CodeUnsafeManagedState, errors.New("background operation lock changed while opening"))
			}
		}
	}
	if err != nil {
		return nil, coded(CodeOperationActive, errors.New("another background operation is active"))
	}
	if created {
		if err := ownerfs.RestrictFile(lock); err != nil {
			file.Close()
			return nil, coded(CodeUnsafeManagedState, err)
		}
	}
	info, err = file.Stat()
	pathInfo, pathErr := ownerfs.ValidateRegular(lock, 64)
	if err != nil || pathErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, pathInfo) {
		file.Close()
		return nil, coded(CodeUnsafeManagedState, errors.New("background operation lock is unsafe"))
	}
	if err := tryOperationLock(file); err != nil {
		file.Close()
		return nil, coded(CodeOperationActive, errors.New("another background operation is active"))
	}
	return func() { _ = releaseOperationLock(file); _ = file.Close() }, nil
}

func (c *controller) hasManagedState() (bool, error) {
	info, err := os.Lstat(c.backgroundDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, coded(CodeUnsafeManagedState, errors.New("background area is unsafe"))
	}
	marker, err := readSmallRegular(filepath.Join(c.backgroundDir, markerFile), 128)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(c.backgroundDir)
		if readErr != nil {
			return false, coded(CodeUnsafeManagedState, readErr)
		}
		for _, entry := range entries {
			if entry.Name() != lockDirectory {
				return false, coded(CodeUnsafeManagedState, errors.New("unmanaged background area is not empty"))
			}
		}
		return false, nil
	}
	if err != nil || string(marker) != backgroundMarker+"\n" {
		return false, coded(CodeUnsafeManagedState, errors.New("background marker is invalid"))
	}
	return true, nil
}

func (c *controller) initializeManagedState(settings Settings, registration registration) error {
	if err := writeSettings(filepath.Join(c.backgroundDir, settingsFile), settings); err != nil {
		return coded(CodeUnsafeManagedState, err)
	}
	if err := writePrivateAtomic(filepath.Join(c.backgroundDir, registration.fileName), registration.content); err != nil {
		return coded(CodeUnsafeManagedState, err)
	}
	if err := writePrivateAtomic(filepath.Join(c.backgroundDir, markerFile), []byte(backgroundMarker+"\n")); err != nil {
		return coded(CodeUnsafeManagedState, err)
	}
	return nil
}

func (c *controller) recoverPartialEnable(registration registration) error {
	allowed := map[string]int64{
		lockDirectory:                   64,
		settingsFile:                    maxSettingsBytes,
		settingsFile + ".next":          maxSettingsBytes,
		registration.fileName:           64 << 10,
		registration.fileName + ".next": 64 << 10,
		markerFile + ".next":            128,
	}
	entries, err := os.ReadDir(c.backgroundDir)
	if err != nil {
		return coded(CodeUnsafeManagedState, err)
	}
	for _, entry := range entries {
		limit, accepted := allowed[entry.Name()]
		if !accepted {
			return coded(CodeUnsafeManagedState, fmt.Errorf("unknown partial background entry: %s", entry.Name()))
		}
		if entry.Name() == lockDirectory {
			continue
		}
		file, err := openRegular(filepath.Join(c.backgroundDir, entry.Name()), limit)
		if err != nil {
			return coded(CodeUnsafeManagedState, errors.New("partial background entry is unsafe"))
		}
		file.Close()
		if err := os.Remove(filepath.Join(c.backgroundDir, entry.Name())); err != nil {
			return coded(CodeUnsafeManagedState, err)
		}
	}
	return nil
}

func (c *controller) loadManaged() (Settings, registration, error) {
	managed, err := c.hasManagedState()
	if err != nil {
		return Settings{}, registration{}, err
	}
	if !managed {
		return Settings{}, registration{}, coded(CodeNotRegistered, errors.New("background profile is not registered"))
	}
	settings, err := readSettings(filepath.Join(c.backgroundDir, settingsFile))
	if err != nil {
		return Settings{}, registration{}, coded(CodeUnsafeManagedState, err)
	}
	if pathWithin(c.installRoot, settings.StateDir) || pathWithin(settings.StateDir, c.installRoot) {
		return Settings{}, registration{}, coded(CodeUnsafeManagedState, errors.New("persisted state directory overlaps program files"))
	}
	definition, err := c.adapter.registration(settings)
	if err != nil {
		return Settings{}, registration{}, coded(CodeUnsafeManagedState, err)
	}
	if err := c.validateManagedFiles(definition); err != nil {
		return Settings{}, registration{}, err
	}
	return settings, definition, nil
}

func (c *controller) validateManagedFiles(registration registration) error {
	allowed := map[string]bool{markerFile: true, settingsFile: true, registration.fileName: true, lockDirectory: true, disablingFile: true}
	entries, err := os.ReadDir(c.backgroundDir)
	if err != nil {
		return coded(CodeUnsafeManagedState, err)
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return coded(CodeUnsafeManagedState, fmt.Errorf("unknown background entry: %s", entry.Name()))
		}
		if entry.Name() == lockDirectory {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return coded(CodeUnsafeManagedState, errors.New("background entry is not a regular file"))
		}
	}
	contents, err := readSmallRegular(filepath.Join(c.backgroundDir, registration.fileName), 64<<10)
	if err != nil || string(contents) != string(registration.content) {
		return coded(CodeRegistrationMismatch, errors.New("managed registration template differs"))
	}
	return nil
}

func pathWithin(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (c *controller) removeManagedState(registration registration) error {
	for _, name := range []string{markerFile, settingsFile, registration.fileName, disablingFile} {
		if err := os.Remove(filepath.Join(c.backgroundDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return coded(CodeUnsafeManagedState, err)
		}
	}
	return nil
}
