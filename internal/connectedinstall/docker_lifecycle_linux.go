//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func inspectInstalledRichProcessesDocker() (connectedprofile.RichProcessesDockerConfig, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return connectedprofile.RichProcessesDockerConfig{}, ErrUnsafe
	}
	c, err := connectedprofile.LoadRichProcessesDockerConfig()
	if err != nil {
		return c, ErrUnsafe
	}
	policy, err := connectedprofile.LoadRichProcessesDockerPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesDockerBinding(c, policy) != nil {
		return c, ErrUnsafe
	}
	if exactMembers(connectedprofile.RichProcessesDockerConfigDirectory, []string{"credentials", "installed.json", "preparing.json", "collection-policy.json"}) != nil || exactMembers(connectedprofile.RichProcessesDockerStateDirectory, []string{"collector-status", "docker-frame", "enrollment", "handoff", "ledger", "reader-root", "reader-status", "uploader-root", "uploader-status"}) != nil {
		return c, ErrRecovery
	}
	preparing, err := connectedprofile.ReadRootFileMode(connectedprofile.RichProcessesDockerConfigDirectory+"/preparing.json", 1024, 0600)
	want, _ := dockerPreparing(c.ServerID, c.ArtifactSHA256)
	if err != nil || !bytes.Equal(preparing, want) {
		return c, ErrUnsafe
	}
	release := connectedprofile.RichProcessesDockerReleaseDirectory + "/" + c.ArtifactSHA256
	if _, err := connectedbundle.VerifyRichProcessesDockerDirectory(release, c.ArtifactSHA256); err != nil {
		return c, ErrUnsafe
	}
	self, err := os.Stat("/proc/self/exe")
	installed, other := os.Stat(release + "/observer-connected-rich-docker-install")
	if err != nil || other != nil || !os.SameFile(self, installed) {
		return c, ErrUnsafe
	}
	layout := dockerLayout(c)
	if auditInstalledDirectories(layout, connectedprofile.RichProcessesDockerStateDirectory, c.UploaderUID, c.UploaderGID) != nil {
		return c, ErrUnsafe
	}
	units, err := connectedunits.RenderRichProcessesDockerUnits(c)
	if err != nil || auditInstalledRenderedFiles(connectedprofile.RichProcessesDockerStateDirectory, connectedunits.RichProcessesDockerCollectorUnit, connectedunits.RichProcessesDockerUploaderUnit, connectedunits.Units{Collector: units.Collector, Uploader: units.Uploader}, c.Addresses) != nil || connectedruntime.CheckInstalledRichProcessesDockerUploaderScaffold() != nil || connectedruntime.CheckInstalledRichProcessesDockerReaderScaffold() != nil {
		return c, ErrUnsafe
	}
	readerUnit, err := connectedprofile.ReadRootFileMode("/etc/systemd/system/"+connectedunits.RichProcessesDockerReaderUnit, connectedunits.MaxRenderedBytes, 0644)
	if err != nil || !bytes.Equal(readerUnit, units.Reader) || auditDockerInstalledDirectories(layout) != nil {
		return c, ErrUnsafe
	}
	return c, nil
}

func auditDockerInstalledDirectories(l dockerInstallLayout) error {
	for _, role := range dockerDirectories {
		spec, err := l.directory(role)
		if err != nil {
			return ErrUnsafe
		}
		parent, err := rootDirectory(spec.parent)
		if err != nil {
			return ErrUnsafe
		}
		fd, err := unix.Openat(int(parent.Fd()), spec.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		parent.Close()
		if err != nil {
			return ErrUnsafe
		}
		d := os.NewFile(uintptr(fd), "installed-reader-directory")
		valid := exactDirectory(d, spec)
		closeErr := d.Close()
		if !valid || closeErr != nil {
			return ErrUnsafe
		}
	}
	return nil
}

func StopRichProcessesDocker(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return ErrUnsafe
	}
	defer lease.Close()
	if _, err := inspectInstalledRichProcessesDocker(); err != nil {
		return err
	}
	collector, uploader, _, _ := richProcessesDockerPair.units()
	owned := map[string]workerInstance{}
	for _, unit := range []string{uploader, collector, connectedunits.RichProcessesDockerReaderUnit} {
		state, err := inspectWorker(ctx, unit)
		if err != nil {
			return ErrUnsafe
		}
		owned[unit] = state
	}
	var result error
	for _, unit := range []string{uploader, collector, connectedunits.RichProcessesDockerReaderUnit} {
		if stopCapturedWorker(unit, owned[unit], inspectWorker, workerService) != nil {
			result = ErrRecovery
		}
	}
	return result
}

func StartRichProcessesDocker(ctx context.Context) (result error) {
	phase := "REQUEST"
	defer func() {
		if result != nil {
			result = &lifecycleError{phase, result}
		}
	}()
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	phase = "LEASE"
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return ErrUnsafe
	}
	defer lease.Close()
	phase = "INSTALLED_STATE"
	c, err := inspectInstalledRichProcessesDocker()
	if err != nil {
		return err
	}
	phase = "HOST_PROFILE"
	if preflight(ctx) != nil {
		return ErrUnsafe
	}
	phase = "PRINCIPALS"
	if auditDockerPrincipals(ctx, c) != nil {
		return ErrUnsafe
	}
	phase = "SERVICE_STATE"
	collector, uploader, _, _ := richProcessesDockerPair.units()
	for _, unit := range []string{collector, uploader, connectedunits.RichProcessesDockerReaderUnit} {
		w, err := inspectWorker(ctx, unit)
		if err != nil || w.PID != 0 || (w.Active != "inactive" && w.Active != "failed") {
			return ErrUnsafe
		}
	}
	phase = "READER_SOCKET"
	socket, err := captureDockerSocket(c.SocketGID)
	if err != nil {
		return ErrUnsafe
	}
	defer socket.Close()
	phase = "READER_STARTUP"
	reader, err := startDockerReaderWith(ctx, c, systemDockerReaderOperations(c, socket))
	if err != nil {
		return err
	}
	owned := map[string]workerInstance{connectedunits.RichProcessesDockerReaderUnit: reader}
	defer func() {
		if result != nil {
			for _, unit := range []string{uploader, collector, connectedunits.RichProcessesDockerReaderUnit} {
				if w, ok := owned[unit]; ok {
					if stopCapturedWorker(unit, w, inspectWorker, workerService) != nil {
						phase = "ACTIVATION_CLEANUP"
						result = ErrRecovery
					}
				}
			}
		}
	}()
	stillReader := func() bool {
		return sameWorker(ctx, connectedunits.RichProcessesDockerReaderUnit, reader) && sameDockerSocket(socket)
	}
	phase = "RUNTIME_GATE"
	ops := dockerActivationOperations(c)
	installed := ops.installed
	ops.installed = func() (connectedprofile.Config, error) {
		if !stillReader() {
			return connectedprofile.Config{}, ErrUnsafe
		}
		return installed()
	}
	ops.started = func(unit string, w workerInstance) { owned[unit] = w }
	if err := activateStoppedWith(ctx, dockerWorkerBinding(c), ops); err != nil {
		return err
	}
	phase = "FINAL_AUDIT"
	if ctx.Err() != nil || !stillReader() {
		return ErrUnsafe
	}
	return nil
}

func dockerActivationOperations(c connectedprofile.RichProcessesDockerConfig) activationOperations {
	initial, _ := connectedprofile.EncodeRichProcessesDockerConfig(c)
	base, _ := connectedprofile.Encode(dockerWorkerBinding(c))
	return activationOperations{pair: richProcessesDockerPair,
		configBytes: func(b connectedprofile.Config) ([]byte, error) {
			got, err := connectedprofile.Encode(b)
			if err != nil || !bytes.Equal(got, base) {
				return nil, ErrUnsafe
			}
			return bytes.Clone(initial), nil
		},
		inspect: inspectWorker, service: workerService, audit: auditDockerActivationProcess,
		pin: func(gid uint32) (activationRecords, error) {
			d, err := pinActivationForPair(gid, richProcessesDockerPair)
			if err != nil {
				return nil, err
			}
			return pinnedActivationRecords{directory: d, gid: gid, pair: richProcessesDockerPair}, nil
		},
		clearResponse: func(b connectedprofile.Config) error {
			return clearActivationResponseForPair(b, richProcessesDockerPair)
		},
		response: func(b connectedprofile.Config) (connectedactivation.Response, error) {
			return readActivationResponseForPair(b, richProcessesDockerPair)
		},
		status: func(name string, uid uint32) (connectedstatus.Record, error) {
			return readDockerStatusRecord(c, name, uid)
		},
		probes: func(ctx context.Context) (activationProbes, error) { return newProbePair(ctx) }, policy: connectedpolicy.ValidateEffective,
		installed: func() (connectedprofile.Config, error) {
			actual, err := inspectInstalledRichProcessesDocker()
			if err != nil {
				return connectedprofile.Config{}, err
			}
			encoded, _ := connectedprofile.EncodeRichProcessesDockerConfig(actual)
			if !bytes.Equal(initial, encoded) {
				return connectedprofile.Config{}, ErrUnsafe
			}
			return dockerWorkerBinding(actual), nil
		},
		random: rand.Read, wait: activationWait,
	}
}

func auditDockerActivationProcess(ctx context.Context, pid int, executable string) (connectedprocess.Identity, error) {
	before, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil {
		return before, ErrUnsafe
	}
	c, err := inspectInstalledRichProcessesDocker()
	if err != nil || executable != richProcessesDockerPair.uploaderExecutable(c.ArtifactSHA256) {
		return before, ErrUnsafe
	}
	sources := map[string]string{
		"/bin/observer-connected-rich-docker-uploader":                 executable,
		"/etc/home-lab-observer-connected-rich/installed.json":         connectedprofile.RichProcessesDockerConfigPath,
		"/etc/home-lab-observer-connected-rich/collection-policy.json": connectedprofile.RichProcessesDockerPolicyPath,
		"/handoff": connectedprofile.RichProcessesDockerStateDirectory + "/handoff", "/activation": "/run/home-lab-observer-connected-rich/activation",
		"/state/ledger": connectedprofile.RichProcessesDockerStateDirectory + "/ledger", "/state/status": connectedprofile.RichProcessesDockerStateDirectory + "/uploader-status",
	}
	if connectedruntime.CheckRichProcessesDockerUploaderNamespace(pid, connectedprofile.RichProcessesDockerStateDirectory+"/uploader-root", sources) != nil {
		return before, ErrUnsafe
	}
	after, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil || before != after {
		return before, ErrUnsafe
	}
	return after, nil
}

func readDockerStatusRecord(c connectedprofile.RichProcessesDockerConfig, name string, uid uint32) (connectedstatus.Record, error) {
	gid := c.UploaderGID
	if name == "collector-status" && uid == c.CollectorUID {
		gid = c.SharedGID
	} else if name == "reader-status" && uid == c.ReaderUID {
		gid = c.FrameGID
	} else if name != "uploader-status" || uid != c.UploaderUID {
		return connectedstatus.Record{}, ErrUnsafe
	}
	parent, err := rootDirectory(connectedprofile.RichProcessesDockerStateDirectory)
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	defer parent.Close()
	d, err := privateDirectoryAt(parent, name, uid, gid)
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	defer d.Close()
	data, err := activationRecord(d, "status.json", uid, gid, 0600)
	if err != nil {
		return connectedstatus.Record{}, err
	}
	return connectedstatus.Decode(data)
}

type DockerStatus struct {
	State     string                  `json:"state"`
	Collector *connectedstatus.Record `json:"collector,omitempty"`
	Uploader  *connectedstatus.Record `json:"uploader,omitempty"`
	Reader    *connectedstatus.Record `json:"reader,omitempty"`
}

func classifyDockerStatus(collector, uploader, reader workerInstance, r DockerStatus, now time.Time) DockerStatus {
	pair := classifyPairStatus(collector, uploader, r.Collector, r.Uploader, now)
	r.State, r.Collector, r.Uploader = pair.State, pair.Collector, pair.Uploader
	switch {
	case r.State == "RECOVERY_REQUIRED" || reader.Active == "failed":
		r.State = "RECOVERY_REQUIRED"
	case reader.Active == "activating" || reader.Active == "deactivating":
		r.State = "STARTING"
	case reader.Active == "inactive" && r.State == "STOPPED":
	case reader.Active != "active" || r.State == "STOPPED":
		r.State = "RECOVERY_REQUIRED"
	case r.Reader == nil || r.Reader.InvocationID != reader.Invocation:
		r.State = "STARTING"
	case r.Reader.State != "COLLECTING" && r.Reader.State != "SOURCE_UNAVAILABLE":
		r.State = "RECOVERY_REQUIRED"
	}
	return r
}

func ReadRichProcessesDockerStatus(ctx context.Context) (DockerStatus, error) {
	if ctx == nil || ctx.Err() != nil {
		return DockerStatus{}, ErrUnsafe
	}
	c, err := inspectInstalledRichProcessesDocker()
	if err != nil {
		return DockerStatus{}, err
	}
	collectorUnit, uploaderUnit, _, _ := richProcessesDockerPair.units()
	collector, err := inspectWorker(ctx, collectorUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	uploader, err := inspectWorker(ctx, uploaderUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	reader, err := inspectWorker(ctx, connectedunits.RichProcessesDockerReaderUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	result := DockerStatus{}
	for _, item := range []struct {
		worker    workerInstance
		directory string
		uid       uint32
		output    **connectedstatus.Record
	}{{collector, "collector-status", c.CollectorUID, &result.Collector}, {uploader, "uploader-status", c.UploaderUID, &result.Uploader}, {reader, "reader-status", c.ReaderUID, &result.Reader}} {
		if item.worker.Active != "active" {
			continue
		}
		record, err := readDockerStatusRecord(c, item.directory, item.uid)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return DockerStatus{}, ErrUnsafe
		}
		if record.InvocationID == item.worker.Invocation {
			*item.output = &record
		}
	}
	currentCollector, err := inspectWorker(ctx, collectorUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	currentUploader, err := inspectWorker(ctx, uploaderUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	currentReader, err := inspectWorker(ctx, connectedunits.RichProcessesDockerReaderUnit)
	if err != nil {
		return DockerStatus{}, ErrUnsafe
	}
	if currentCollector != collector || currentUploader != uploader || currentReader != reader {
		return DockerStatus{State: "STARTING"}, nil
	}
	return classifyDockerStatus(collector, uploader, reader, result, time.Now().UTC()), nil
}
