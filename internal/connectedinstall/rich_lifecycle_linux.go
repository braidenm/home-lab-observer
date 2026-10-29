//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"strings"
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

func inspectInstalledRichProcesses() (connectedprofile.RichProcessesConfig, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return connectedprofile.RichProcessesConfig{}, ErrUnsafe
	}
	c, err := connectedprofile.LoadRichProcessesConfig()
	if err != nil {
		return c, ErrUnsafe
	}
	policy, err := connectedprofile.LoadRichProcessesPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesBinding(c, policy) != nil {
		return c, ErrUnsafe
	}
	if exactMembers(connectedprofile.RichProcessesConfigDirectory, []string{"credentials", "installed.json", "preparing.json", "collection-policy.json"}) != nil || exactMembers(connectedprofile.RichProcessesStateDirectory, []string{"collector-status", "enrollment", "handoff", "ledger", "uploader-root", "uploader-status"}) != nil {
		return c, ErrRecovery
	}
	preparing, err := connectedprofile.ReadRootFileMode(connectedprofile.RichProcessesConfigDirectory+"/preparing.json", 1024, 0600)
	want, _ := richPreparing(c.ServerID, c.ArtifactSHA256)
	if err != nil || !bytes.Equal(preparing, want) {
		return c, ErrUnsafe
	}
	release := connectedprofile.RichProcessesReleaseDirectory + "/" + c.ArtifactSHA256
	if _, err := connectedbundle.VerifyRichProcessesDirectory(release, c.ArtifactSHA256); err != nil {
		return c, ErrUnsafe
	}
	self, err := os.Stat("/proc/self/exe")
	installed, other := os.Stat(release + "/observer-connected-rich-install")
	if err != nil || other != nil || !os.SameFile(self, installed) {
		return c, ErrUnsafe
	}
	layout := richInstallLayout{installLayout{digest: c.ArtifactSHA256, collectorUID: c.CollectorUID, uploaderUID: c.UploaderUID, uploaderGID: c.UploaderGID, sharedGID: c.SharedGID}}
	if auditInstalledDirectories(layout, connectedprofile.RichProcessesStateDirectory, c.UploaderUID, c.UploaderGID) != nil {
		return c, ErrUnsafe
	}
	units, err := connectedunits.RenderRichProcessesUnits(c)
	if err != nil || auditInstalledRenderedFiles(connectedprofile.RichProcessesStateDirectory, connectedunits.RichProcessesCollectorUnit, connectedunits.RichProcessesUploaderUnit, connectedunits.Units(units), c.Addresses) != nil || connectedruntime.CheckInstalledRichProcessesUploaderScaffold() != nil {
		return c, ErrUnsafe
	}
	return c, nil
}

type directoryLayout interface {
	directory(directoryRole) (directorySpec, error)
}

func auditInstalledDirectories(layout directoryLayout, stateRoot string, uploaderUID, uploaderGID uint32) error {
	for _, role := range []directoryRole{dirRelease, dirHandoff, dirCollectorStatus, dirUploaderStatus, dirUploaderRoot, dirCredentials} {
		spec, err := layout.directory(role)
		if err != nil {
			return ErrUnsafe
		}
		var d *os.File
		if spec.uid == 0 {
			d, err = rootDirectory(spec.parent + "/" + spec.name)
		} else {
			parent, e := rootDirectory(spec.parent)
			if e != nil {
				return ErrUnsafe
			}
			fd, e := unix.Openat(int(parent.Fd()), spec.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			parent.Close()
			err = e
			if e == nil {
				d = os.NewFile(uintptr(fd), "installed-directory")
			}
		}
		if err != nil {
			return ErrUnsafe
		}
		valid := exactDirectory(d, spec)
		d.Close()
		if !valid {
			return ErrUnsafe
		}
	}
	parent, err := rootDirectory(stateRoot)
	if err != nil {
		return ErrUnsafe
	}
	ledger, err := privateDirectoryAt(parent, "ledger", uploaderUID, uploaderGID)
	parent.Close()
	if err != nil {
		return ErrUnsafe
	}
	return ledger.Close()
}

func auditInstalledRenderedFiles(stateRoot, collector, uploader string, units connectedunits.Units, addresses []string) error {
	var hosts strings.Builder
	for _, address := range addresses {
		hosts.WriteString(address + " " + connectedprofile.Hostname + "\n")
	}
	for _, item := range []struct {
		path string
		want []byte
	}{
		{"/etc/systemd/system/" + collector, units.Collector}, {"/etc/systemd/system/" + uploader, units.Uploader},
		{stateRoot + "/uploader-root/etc/hosts", []byte(hosts.String())}, {stateRoot + "/uploader-root/etc/resolv.conf", []byte("# No runtime DNS resolver\n")}, {stateRoot + "/uploader-root/etc/nsswitch.conf", []byte("hosts: files\n")},
	} {
		actual, err := connectedprofile.ReadRootFileMode(item.path, connectedunits.MaxRenderedBytes, 0644)
		if err != nil || !bytes.Equal(actual, item.want) {
			return ErrUnsafe
		}
	}
	if _, err := connectedprofile.ReadRootFileMode(stateRoot+"/uploader-root/etc/ssl/certs/ca-certificates.crt", 1<<20, 0644); err != nil {
		return ErrUnsafe
	}
	return nil
}

func StartRichProcesses(ctx context.Context) (result error) {
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
	c, err := inspectInstalledRichProcesses()
	if err != nil {
		return err
	}
	phase = "HOST_PROFILE"
	if preflight(ctx) != nil {
		return ErrUnsafe
	}
	phase = "PRINCIPALS"
	if auditPrincipalsForPair(ctx, richWorkerBinding(c), richProcessesPair) != nil {
		return ErrUnsafe
	}
	phase = "SERVICE_STATE"
	collector, uploader, _, _ := richProcessesPair.units()
	for _, unit := range []string{uploader, collector} {
		state, err := inspectWorker(ctx, unit)
		if err != nil || state.PID != 0 || (state.Active != "inactive" && state.Active != "failed") {
			return ErrUnsafe
		}
	}
	phase = "RUNTIME_GATE"
	return activateStoppedWith(ctx, richWorkerBinding(c), richActivationOperations(c))
}

func StopRichProcesses(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return ErrUnsafe
	}
	defer lease.Close()
	if _, err := inspectInstalledRichProcesses(); err != nil {
		return err
	}
	collector, uploader, _, _ := richProcessesPair.units()
	owned := map[string]workerInstance{}
	for _, unit := range []string{uploader, collector} {
		state, err := inspectWorker(ctx, unit)
		if err != nil {
			return ErrUnsafe
		}
		owned[unit] = state
	}
	var result error
	for _, unit := range []string{uploader, collector} {
		if stopCapturedWorker(unit, owned[unit], inspectWorker, workerService) != nil {
			result = ErrRecovery
		}
	}
	return result
}

func richActivationOperations(c connectedprofile.RichProcessesConfig) activationOperations {
	initial, _ := connectedprofile.EncodeRichProcessesConfig(c)
	base, _ := connectedprofile.Encode(richWorkerBinding(c))
	return activationOperations{pair: richProcessesPair,
		configBytes: func(b connectedprofile.Config) ([]byte, error) {
			got, err := connectedprofile.Encode(b)
			if err != nil || !bytes.Equal(got, base) {
				return nil, ErrUnsafe
			}
			return bytes.Clone(initial), nil
		},
		inspect: inspectWorker, service: workerService, audit: auditRichActivationProcess,
		pin: func(gid uint32) (activationRecords, error) {
			d, err := pinActivationForPair(gid, richProcessesPair)
			if err != nil {
				return nil, err
			}
			return pinnedActivationRecords{directory: d, gid: gid, pair: richProcessesPair}, nil
		},
		clearResponse: func(b connectedprofile.Config) error { return clearActivationResponseForPair(b, richProcessesPair) },
		response: func(b connectedprofile.Config) (connectedactivation.Response, error) {
			return readActivationResponseForPair(b, richProcessesPair)
		},
		status: func(name string, uid uint32) (connectedstatus.Record, error) {
			return readRichStatusRecord(c, name, uid)
		},
		probes: func(ctx context.Context) (activationProbes, error) { return newProbePair(ctx) }, policy: connectedpolicy.ValidateEffective,
		installed: func() (connectedprofile.Config, error) {
			actual, err := inspectInstalledRichProcesses()
			if err != nil {
				return connectedprofile.Config{}, err
			}
			encoded, _ := connectedprofile.EncodeRichProcessesConfig(actual)
			if !bytes.Equal(initial, encoded) {
				return connectedprofile.Config{}, ErrUnsafe
			}
			return richWorkerBinding(actual), nil
		},
		random: rand.Read, wait: activationWait,
	}
}

func auditRichActivationProcess(ctx context.Context, pid int, executable string) (connectedprocess.Identity, error) {
	before, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil {
		return before, ErrUnsafe
	}
	c, err := inspectInstalledRichProcesses()
	if err != nil || executable != richProcessesPair.uploaderExecutable(c.ArtifactSHA256) {
		return before, ErrUnsafe
	}
	sources := map[string]string{
		"/bin/observer-connected-rich-uploader":                        executable,
		"/etc/home-lab-observer-connected-rich/installed.json":         connectedprofile.RichProcessesConfigPath,
		"/etc/home-lab-observer-connected-rich/collection-policy.json": connectedprofile.RichProcessesPolicyPath,
		"/handoff": connectedprofile.RichProcessesStateDirectory + "/handoff", "/activation": "/run/home-lab-observer-connected-rich/activation",
		"/state/ledger": connectedprofile.RichProcessesStateDirectory + "/ledger", "/state/status": connectedprofile.RichProcessesStateDirectory + "/uploader-status",
	}
	if connectedruntime.CheckRichProcessesUploaderNamespace(pid, connectedprofile.RichProcessesStateDirectory+"/uploader-root", sources) != nil {
		return before, ErrUnsafe
	}
	after, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil || before != after {
		return before, ErrUnsafe
	}
	return after, nil
}

func readRichStatusRecord(c connectedprofile.RichProcessesConfig, name string, uid uint32) (connectedstatus.Record, error) {
	gid := c.UploaderGID
	if name == "collector-status" && uid == c.CollectorUID {
		gid = c.SharedGID
	} else if name != "uploader-status" || uid != c.UploaderUID {
		return connectedstatus.Record{}, ErrUnsafe
	}
	parent, err := rootDirectory(connectedprofile.RichProcessesStateDirectory)
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

func ReadRichProcessesStatus(ctx context.Context) (Status, error) {
	if ctx == nil || ctx.Err() != nil {
		return Status{}, ErrUnsafe
	}
	c, err := inspectInstalledRichProcesses()
	if err != nil {
		return Status{}, err
	}
	collectorUnit, uploaderUnit, _, _ := richProcessesPair.units()
	collector, err := inspectWorker(ctx, collectorUnit)
	if err != nil {
		return Status{}, ErrUnsafe
	}
	uploader, err := inspectWorker(ctx, uploaderUnit)
	if err != nil {
		return Status{}, ErrUnsafe
	}
	result := Status{}
	for _, item := range []struct {
		worker    workerInstance
		directory string
		uid       uint32
		output    **connectedstatus.Record
	}{{collector, "collector-status", c.CollectorUID, &result.Collector}, {uploader, "uploader-status", c.UploaderUID, &result.Uploader}} {
		if item.worker.Active != "active" {
			continue
		}
		record, err := readRichStatusRecord(c, item.directory, item.uid)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Status{}, ErrUnsafe
		}
		if record.InvocationID == item.worker.Invocation {
			*item.output = &record
		}
	}
	currentCollector, err := inspectWorker(ctx, collectorUnit)
	if err != nil {
		return Status{}, ErrUnsafe
	}
	currentUploader, err := inspectWorker(ctx, uploaderUnit)
	if err != nil {
		return Status{}, ErrUnsafe
	}
	if currentCollector != collector || currentUploader != uploader {
		return Status{State: "STARTING"}, nil
	}
	return classifyPairStatus(collector, uploader, result.Collector, result.Uploader, time.Now().UTC()), nil
}
