//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type Status struct {
	State     string                  `json:"state"`
	Collector *connectedstatus.Record `json:"collector,omitempty"`
	Uploader  *connectedstatus.Record `json:"uploader,omitempty"`
}

// Start never enrolls, resets delivery state or enables boot activation. The
// installed binary and both fixed services must match the exact released bundle.
func Start(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return ErrUnsafe
	}
	defer lease.Close()
	c, err := inspectInstalled()
	if err != nil || preflight(ctx) != nil || auditPrincipals(ctx, c) != nil {
		return ErrUnsafe
	}
	if err := stoppedOwnedWorkers(ctx); err != nil {
		return err
	}
	return activateStopped(ctx, c)
}

// Stop targets only the installed pair after validating exact immutable inputs.
func Stop(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return ErrUnsafe
	}
	defer lease.Close()
	if _, err := inspectInstalled(); err != nil {
		return err
	}
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if _, err := inspectWorker(ctx, unit); err != nil {
			return ErrUnsafe
		}
	}
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if workerService(ctx, "stop", unit) != nil {
			return ErrRecovery
		}
		state, err := inspectWorker(ctx, unit)
		if err != nil || state.PID != 0 || state.Active != "inactive" {
			return ErrRecovery
		}
	}
	return nil
}

func stoppedOwnedWorkers(ctx context.Context) error {
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		state, err := inspectWorker(ctx, unit)
		if err != nil || state.PID != 0 || (state.Active != "inactive" && state.Active != "failed") {
			return ErrUnsafe
		}
	}
	return nil
}

func workerService(ctx context.Context, verb, unit string) error {
	if (unit != collectorUnit && unit != uploaderUnit) || (verb != "start" && verb != "stop") {
		return ErrUnsafe
	}
	_, err := runTool(ctx, toolSystemctl, []string{verb, "--", unit}, nil, 1024)
	return err
}

func inspectInstalled() (connectedprofile.Config, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return connectedprofile.Config{}, ErrUnsafe
	}
	data, err := connectedprofile.ReadRootFileMode(connectedprofile.ConfigPath, connectedprofile.MaxConfigBytes, 0644)
	c, decodeErr := connectedprofile.Decode(data)
	if err != nil || decodeErr != nil {
		return connectedprofile.Config{}, ErrUnsafe
	}
	if exactMembers(connectedprofile.ConfigDirectory, []string{"credentials", "installed.json", "preparing.json"}) != nil ||
		exactMembers(connectedprofile.StateDirectory, []string{"collector-status", "enrollment", "handoff", "ledger", "uploader-root", "uploader-status"}) != nil {
		return c, ErrRecovery
	}
	preparing, err := connectedprofile.ReadRootFileMode(connectedprofile.ConfigDirectory+"/preparing.json", 1024, 0600)
	wantPreparing, _ := json.Marshal(struct {
		Version  string `json:"version"`
		State    string `json:"state"`
		ServerID string `json:"server_id"`
		Digest   string `json:"manifest_sha256"`
	}{"observer-connected-preparing/v1", "PREPARING", c.ServerID, c.ArtifactSHA256})
	if err != nil || !bytes.Equal(preparing, wantPreparing) {
		return c, ErrUnsafe
	}
	release := connectedprofile.ReleaseDirectory + "/" + c.ArtifactSHA256
	if _, err := connectedbundle.VerifyDirectory(release, c.ArtifactSHA256); err != nil {
		return c, ErrUnsafe
	}
	// Start must run the installed installer, rather than unrelated downloaded code.
	self, err := os.Stat("/proc/self/exe")
	installed, installedErr := os.Stat(release + "/observer-connected-install")
	if err != nil || installedErr != nil || !os.SameFile(self, installed) {
		return c, ErrUnsafe
	}
	layout := installLayout{digest: c.ArtifactSHA256, collectorUID: c.CollectorUID, uploaderUID: c.UploaderUID, uploaderGID: c.UploaderGID, sharedGID: c.SharedGID}
	for _, role := range []directoryRole{dirRelease, dirHandoff, dirCollectorStatus, dirUploaderStatus, dirUploaderRoot, dirCredentials} {
		spec, err := layout.directory(role)
		if err != nil {
			return c, ErrUnsafe
		}
		var d *os.File
		if spec.uid == 0 {
			d, err = rootDirectory(spec.parent + "/" + spec.name)
		} else {
			parent, parentErr := rootDirectory(spec.parent)
			if parentErr != nil {
				return c, ErrUnsafe
			}
			fd, openErr := unix.Openat(int(parent.Fd()), spec.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			parent.Close()
			err = openErr
			if err == nil {
				d = os.NewFile(uintptr(fd), "installed-directory")
			}
		}
		if err != nil {
			return c, ErrUnsafe
		}
		valid := exactDirectory(d, spec)
		d.Close()
		if !valid {
			return c, ErrUnsafe
		}
	}
	parent, err := rootDirectory(connectedprofile.StateDirectory)
	if err != nil {
		return c, ErrUnsafe
	}
	ledger, err := privateDirectoryAt(parent, "ledger", c.UploaderUID, c.UploaderGID)
	parent.Close()
	if err != nil {
		return c, ErrUnsafe
	}
	ledger.Close()
	units, err := connectedunits.RenderUnits(c)
	if err != nil {
		return c, ErrUnsafe
	}
	var hosts strings.Builder
	for _, address := range c.Addresses {
		hosts.WriteString(address + " " + connectedprofile.Hostname + "\n")
	}
	for _, item := range []struct {
		path string
		want []byte
	}{
		{"/etc/systemd/system/" + collectorUnit, units.Collector},
		{"/etc/systemd/system/" + uploaderUnit, units.Uploader},
		{connectedprofile.StateDirectory + "/uploader-root/etc/hosts", []byte(hosts.String())},
		{connectedprofile.StateDirectory + "/uploader-root/etc/resolv.conf", []byte("# No runtime DNS resolver\n")},
		{connectedprofile.StateDirectory + "/uploader-root/etc/nsswitch.conf", []byte("hosts: files\n")},
	} {
		actual, err := connectedprofile.ReadRootFileMode(item.path, connectedunits.MaxRenderedBytes, 0644)
		if err != nil || !bytes.Equal(actual, item.want) {
			return c, ErrUnsafe
		}
	}
	if _, err := connectedprofile.ReadRootFileMode(connectedprofile.StateDirectory+"/uploader-root/etc/ssl/certs/ca-certificates.crt", 1<<20, 0644); err != nil {
		return c, ErrUnsafe
	}
	return c, nil
}

func exactMembers(path string, expected []string) error {
	d, err := rootDirectory(path)
	if err != nil {
		return ErrUnsafe
	}
	defer d.Close()
	names, err := d.Readdirnames(len(expected) + 1)
	if err != nil && err != io.EOF {
		return ErrUnsafe
	}
	slices.Sort(names)
	want := append([]string(nil), expected...)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		return ErrRecovery
	}
	return nil
}

func readStatusRecord(name string, uid uint32) (connectedstatus.Record, error) {
	c, err := connectedprofile.Load()
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	gid := c.UploaderGID
	if name == "collector-status" && uid == c.CollectorUID {
		gid = c.SharedGID
	} else if name != "uploader-status" || uid != c.UploaderUID {
		return connectedstatus.Record{}, ErrUnsafe
	}
	parent, err := rootDirectory(connectedprofile.StateDirectory)
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

func ReadStatus(ctx context.Context) (Status, error) {
	if ctx == nil || ctx.Err() != nil {
		return Status{}, ErrUnsafe
	}
	c, err := inspectInstalled()
	if err != nil {
		return Status{}, err
	}
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
	}{
		{collector, "collector-status", c.CollectorUID, &result.Collector},
		{uploader, "uploader-status", c.UploaderUID, &result.Uploader},
	} {
		if item.worker.Active != "active" {
			continue
		}
		record, err := readStatusRecord(item.directory, item.uid)
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
	if collector != currentCollector || uploader != currentUploader {
		return Status{State: "STARTING"}, nil
	}
	return classifyPairStatus(collector, uploader, result.Collector, result.Uploader, time.Now().UTC()), nil
}

func classifyPairStatus(collector, uploader workerInstance, collected, uploaded *connectedstatus.Record, now time.Time) Status {
	result := Status{Collector: collected, Uploader: uploaded}
	if collector.Active == "failed" || uploader.Active == "failed" {
		result.State = "RECOVERY_REQUIRED"
	} else if collector.Active == "activating" || collector.Active == "deactivating" || uploader.Active == "activating" || uploader.Active == "deactivating" {
		result.State = "STARTING"
	} else if collector.Active == "inactive" && uploader.Active == "inactive" {
		result.State = "STOPPED"
	} else if collector.Active != "active" || uploader.Active != "active" {
		result.State = "RECOVERY_REQUIRED"
	} else if collected == nil || uploaded == nil || collected.InvocationID != collector.Invocation || uploaded.InvocationID != uploader.Invocation {
		result.State = "STARTING"
	} else if collected.State != "COLLECTING" {
		result.State = "RECOVERY_REQUIRED"
	} else if uploaded.State == "STOPPED" {
		result.State = "STARTING"
	} else {
		record := *uploaded
		if record.State == "ACKNOWLEDGED_FRESH" && (record.CollectedAt.IsZero() || now.Sub(record.CollectedAt) > connectedstatus.FreshnessWindow || record.CollectedAt.After(now.Add(uploadstate.MaxFuture))) {
			record.State = "ACKNOWLEDGED_STALE"
		}
		result.Uploader = &record
		result.State = record.State
	}
	return result
}
