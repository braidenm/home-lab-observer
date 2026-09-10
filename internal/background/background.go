// Package background manages the observer's opt-in, per-user background profile.
//
// It intentionally exposes fixed lifecycle operations rather than a generic
// service-manager or command-execution surface.
package background

import (
	"context"
	"errors"
	"time"
)

const (
	SchemaVersion     = "observer-background/v1"
	ScopeUserSession  = "USER_SESSION"
	SessionLimitation = "LOGIN_SESSION_REQUIRED"
	GracefulWait      = 35 * time.Second
)

type State string

const (
	StateManagerUnavailable State = "MANAGER_UNAVAILABLE"
	StateNotRegistered      State = "NOT_REGISTERED"
	StateRegisteredStopped  State = "REGISTERED_STOPPED"
	StateRunning            State = "RUNNING"
	StateUnreachable        State = "UNREACHABLE"
	StateNotReady           State = "NOT_READY"
)

type Readiness string

const (
	ReadinessUnknown     Readiness = "UNKNOWN"
	ReadinessReady       Readiness = "READY"
	ReadinessNotReady    Readiness = "NOT_READY"
	ReadinessUnreachable Readiness = "UNREACHABLE"
)

type DiagnosticsAvailability string

const (
	DiagnosticsUnknown     DiagnosticsAvailability = "UNKNOWN"
	DiagnosticsAvailable   DiagnosticsAvailability = "AVAILABLE"
	DiagnosticsUnavailable DiagnosticsAvailability = "UNAVAILABLE"
	DiagnosticsDisabled    DiagnosticsAvailability = "DISABLED"
)

type ProbeStatus struct {
	Readiness   Readiness
	Diagnostics DiagnosticsAvailability
}

type Code string

const (
	CodeOK                     Code = "BACKGROUND_OK"
	CodeManagerUnavailable     Code = "BACKGROUND_MANAGER_UNAVAILABLE"
	CodeNotRegistered          Code = "BACKGROUND_NOT_REGISTERED"
	CodeStopped                Code = "BACKGROUND_STOPPED"
	CodeUnreachable            Code = "BACKGROUND_UNREACHABLE"
	CodeNotReady               Code = "BACKGROUND_NOT_READY"
	CodeReadinessUnknown       Code = "BACKGROUND_READINESS_UNKNOWN"
	CodeInvalidSettings        Code = "BACKGROUND_INVALID_SETTINGS"
	CodeUnsafeManagedState     Code = "BACKGROUND_UNSAFE_MANAGED_STATE"
	CodeRegistrationMismatch   Code = "BACKGROUND_REGISTRATION_MISMATCH"
	CodeOperationActive        Code = "BACKGROUND_OPERATION_ACTIVE"
	CodeGracefulStopFailed     Code = "BACKGROUND_GRACEFUL_STOP_FAILED"
	CodeManagerOperationFailed Code = "BACKGROUND_MANAGER_OPERATION_FAILED"
)

type Settings struct {
	StateDir       string   `json:"state_dir"`
	ListenAddress  string   `json:"listen_address"`
	DockerEndpoint string   `json:"docker_endpoint"`
	LogSources     []string `json:"log_sources,omitempty"`
}

type StopOptions struct {
	// Force authorizes the platform manager's destructive termination path.
	// It is never inferred from a graceful-stop timeout.
	Force bool
}

type Status struct {
	State             State                   `json:"state"`
	Code              Code                    `json:"code"`
	Scope             string                  `json:"scope"`
	SessionLimitation string                  `json:"session_limitation"`
	Readiness         Readiness               `json:"readiness"`
	Diagnostics       DiagnosticsAvailability `json:"diagnostics"`
	Registered        bool                    `json:"registered"`
	Running           bool                    `json:"running"`
}

type Manager interface {
	Enable(context.Context, Settings) (Status, error)
	LoadSettings() (Settings, error)
	Start(context.Context) (Status, error)
	Status(context.Context) (Status, error)
	Stop(context.Context, StopOptions) (Status, error)
	Restart(context.Context, StopOptions) (Status, error)
	Disable(context.Context, StopOptions) (Status, error)
}

// GracefulStopper sends the fixed, instance-bound local lifecycle request.
// Implementations must enforce their own request and response bounds.
type GracefulStopper interface {
	RequestStop(context.Context, string) error
}

type GracefulStopFunc func(context.Context, string) error

func (function GracefulStopFunc) RequestStop(ctx context.Context, stateDir string) error {
	return function(ctx, stateDir)
}

// ReadinessProbe checks only the configured loopback observer. It must not
// follow redirects or use proxy settings.
type ReadinessProbe interface {
	Probe(context.Context, Settings) ProbeStatus
}

type Config struct {
	InstallRoot string
	Stopper     GracefulStopper
	Probe       ReadinessProbe
	Timeout     time.Duration
}

type Error struct {
	Code Code
	Err  error
}

func (e *Error) Error() string { return string(e.Code) }
func (e *Error) Unwrap() error { return e.Err }

func IsCode(err error, code Code) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
