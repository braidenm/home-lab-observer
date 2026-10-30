//go:build linux

package connectedinstall

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"golang.org/x/sys/unix"
)

type dockerReaderRecords struct {
	directory *os.File
	gid       uint32
}

func (r dockerReaderRecords) publish(name string, b []byte) error {
	return publishDockerReaderActivation(r.directory, name, b, r.gid)
}
func (r dockerReaderRecords) close() { r.directory.Close() }

// Private deterministic seams only; production callers cannot select paths,
// commands, probes or a different policy/profile through these operations.
type dockerReaderOperations struct {
	inspect       func(context.Context, string) (workerInstance, error)
	service       func(context.Context, string, string) error
	pin           func(uint32) (activationRecords, error)
	clearResponse func() error
	response      func() (connectedactivation.Response, error)
	audit         func(context.Context, workerInstance) error
	status        func() (connectedstatus.Record, error)
	unchanged     func(context.Context, workerInstance) bool
	random        func([]byte) (int, error)
	wait          func(context.Context) error
}

func systemDockerReaderOperations(c connectedprofile.RichProcessesDockerConfig, socket dockerSocketIdentity) dockerReaderOperations {
	return dockerReaderOperations{
		inspect: inspectWorker, service: workerService,
		pin: func(gid uint32) (activationRecords, error) {
			d, err := pinDockerReaderActivation(gid)
			if err != nil {
				return nil, err
			}
			return dockerReaderRecords{d, gid}, nil
		},
		clearResponse: func() error { return clearDockerReaderResponse(c) }, response: func() (connectedactivation.Response, error) { return readDockerReaderResponse(c) },
		audit:  func(ctx context.Context, w workerInstance) error { return auditDockerReaderProcess(ctx, c, w, socket) },
		status: func() (connectedstatus.Record, error) { return readDockerStatusRecord(c, "reader-status", c.ReaderUID) },
		unchanged: func(ctx context.Context, w workerInstance) bool {
			return sameWorker(ctx, connectedunits.RichProcessesDockerReaderUnit, w) && sameDockerSocket(socket)
		},
		random: rand.Read, wait: activationWait,
	}
}

// The only application-visible transition is the existing matching commit.
// Before it, the reader is paused with every primitive socket closed. Root's
// original socket-free descriptor audit is therefore meaningful, not a race.
func startDockerReaderWith(parent context.Context, c connectedprofile.RichProcessesDockerConfig, ops dockerReaderOperations) (owned workerInstance, result error) {
	phase := "READER_REQUEST"
	defer func() {
		if result == nil {
			return
		}
		primary := result
		cleanup, cleanupRequired := result.(*dockerReaderCleanupError)
		if cleanupRequired {
			primary = cleanup.primary
		}
		// Retain only finite classifications, never a provider error or its text.
		cause := ErrUnsafe
		if errors.Is(primary, ErrRecovery) {
			cause = ErrRecovery
		}
		result = &lifecycleError{phase, cause}
		if cleanupRequired {
			result = &dockerReaderCleanupError{result}
		}
	}()
	if parent == nil || parent.Err() != nil || c.Validate() != nil {
		return owned, ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	unit := connectedunits.RichProcessesDockerReaderUnit
	phase = "READER_STATE"
	state, err := ops.inspect(ctx, unit)
	if err != nil || state.PID != 0 || (state.Active != "inactive" && state.Active != "failed") {
		return owned, ErrUnsafe
	}
	phase = "READER_ACTIVATION_PIN"
	d, err := ops.pin(c.FrameGID)
	if err != nil {
		return owned, err
	}
	defer d.close()
	phase = "READER_RESPONSE_CLEAR"
	if err := ops.clearResponse(); err != nil {
		return owned, err
	}
	workers := map[string]workerInstance{}
	defer func() {
		if result != nil {
			if w, ok := workers[unit]; ok {
				if stopCapturedWorker(unit, w, ops.inspect, ops.service) != nil {
					result = &dockerReaderCleanupError{result}
				}
			}
		}
	}()
	phase = "READER_SERVICE_START"
	if err := startActivationWorkerWith(ctx, unit, workers, ops.service, ops.inspect); err != nil {
		return workers[unit], err
	}
	owned = workers[unit]
	// It is still waiting for request.json, before even the primitive checks.
	phase = "READER_AUDIT_BEFORE_REQUEST"
	if ops.audit(ctx, owned) != nil || !ops.unchanged(ctx, owned) {
		return owned, ErrUnsafe
	}
	phase = "READER_REQUEST_BUILD"
	configuration, err := connectedprofile.EncodeRichProcessesDockerReaderStartup(c)
	if err != nil {
		return owned, ErrUnsafe
	}
	hash := sha256.Sum256(configuration)
	nonce := make([]byte, 32)
	if n, err := ops.random(nonce); err != nil || n != len(nonce) {
		return owned, ErrUnsafe
	}
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: hex.EncodeToString(nonce), ArtifactSHA256: c.ArtifactSHA256, ConfigSHA256: hex.EncodeToString(hash[:]), PolicyGeneration: c.PolicyGeneration, IPv4Port: 1024, IPv6Port: 1024}
	b, err := connectedactivation.EncodeRequest(request)
	if err != nil || !ops.unchanged(ctx, owned) {
		return owned, ErrUnsafe
	}
	phase = "READER_REQUEST_PUBLICATION"
	if d.publish("request.json", b) != nil {
		return owned, ErrRecovery
	}
	var response connectedactivation.Response
	phase = "READER_RESPONSE_WAIT"
	for {
		if !ops.unchanged(ctx, owned) {
			return owned, ErrUnsafe
		}
		response, err = ops.response()
		if err == nil {
			phase = "READER_RESPONSE_MATCH"
			if !connectedactivation.MatchResponse(request, owned.Invocation, response) {
				return owned, ErrUnsafe
			}
			break
		}
		if !os.IsNotExist(err) || ops.wait(ctx) != nil {
			return owned, ErrUnsafe
		}
	}
	// The response retains the worker's fresh challenge; it cannot proceed to
	// Engine I/O during the two descriptor passes and fixed namespace inspection.
	phase = "READER_AUDIT_AFTER_RESPONSE"
	if ops.audit(ctx, owned) != nil || !ops.unchanged(ctx, owned) {
		return owned, ErrUnsafe
	}
	phase = "READER_COMMIT_PUBLICATION"
	commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
	b, err = connectedactivation.EncodeCommit(commit)
	if err != nil || ctx.Err() != nil || d.publish("commit.json", b) != nil {
		return owned, ErrRecovery
	}
	phase = "READER_STATUS_WAIT"
	for {
		if !ops.unchanged(ctx, owned) {
			return owned, ErrUnsafe
		}
		r, err := ops.status()
		if err == nil && r.InvocationID == owned.Invocation {
			if r.State == "COLLECTING" || r.State == "SOURCE_UNAVAILABLE" {
				phase = "READER_FINAL_IDENTITY"
				if ctx.Err() != nil || !ops.unchanged(ctx, owned) {
					return owned, ErrUnsafe
				}
				return owned, nil
			}
			if r.State != "STOPPED" {
				return owned, ErrUnsafe
			}
		}
		if err != nil && !os.IsNotExist(err) {
			return owned, ErrUnsafe
		}
		if ops.wait(ctx) != nil {
			return owned, ErrUnsafe
		}
	}
}

// Cleanup uncertainty adds recovery without replacing the original refusal.
// The primary is sanitized by startDockerReaderWith before it leaves that call.
type dockerReaderCleanupError struct{ primary error }

func (e *dockerReaderCleanupError) Error() string        { return "connected_reader_cleanup_required" }
func (e *dockerReaderCleanupError) Unwrap() error        { return e.primary }
func (e *dockerReaderCleanupError) Is(target error) bool { return target == ErrRecovery }

func auditDockerReaderProcess(ctx context.Context, c connectedprofile.RichProcessesDockerConfig, w workerInstance, socket dockerSocketIdentity) error {
	executable := connectedprofile.RichProcessesDockerReleaseDirectory + "/" + c.ArtifactSHA256 + "/observer-connected-rich-docker-reader"
	before, err := connectedprocess.Audit(ctx, w.PID, executable)
	if err != nil || !sameWorker(ctx, connectedunits.RichProcessesDockerReaderUnit, w) || !sameDockerSocket(socket) {
		return ErrUnsafe
	}
	sources := map[string]string{"/bin/observer-connected-rich-docker-reader": executable, connectedprofile.RichProcessesDockerConfigPath: connectedprofile.RichProcessesDockerConfigPath, connectedprofile.RichProcessesDockerPolicyPath: connectedprofile.RichProcessesDockerPolicyPath, "/frame": connectedprofile.RichProcessesDockerFrameDirectory, "/state/status": connectedprofile.RichProcessesDockerReaderStatusDirectory, "/engine/docker.sock": "/run/docker.sock", "/activation": dockerReaderActivationPath}
	if connectedruntime.CheckRichProcessesDockerReaderNamespace(w.PID, connectedprofile.RichProcessesDockerReaderRoot, sources) != nil {
		return ErrUnsafe
	}
	var mounted unix.Stat_t
	if unix.Stat("/proc/"+strconv.Itoa(w.PID)+"/root/engine/docker.sock", &mounted) != nil || !validDockerSocket(mounted, c.SocketGID) || uint64(mounted.Dev) != socket.dev || mounted.Ino != socket.ino {
		return ErrUnsafe
	}
	after, err := connectedprocess.Audit(ctx, w.PID, executable)
	if err != nil || before != after || !sameWorker(ctx, connectedunits.RichProcessesDockerReaderUnit, w) || !sameDockerSocket(socket) {
		return ErrUnsafe
	}
	return nil
}
