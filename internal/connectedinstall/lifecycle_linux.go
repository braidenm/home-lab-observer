//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
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

type preparingRecord struct {
	Version        string `json:"version"`
	State          string `json:"state"`
	ServerID       string `json:"server_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type removalRecord struct {
	Version        string `json:"version"`
	State          string `json:"state"`
	ServerID       string `json:"server_id"`
	ConnectorID    string `json:"connector_id"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

func removalBytes(c connectedprofile.Config, state string) []byte {
	b, _ := json.Marshal(removalRecord{"observer-connected-removal/v1", state, c.ServerID, c.ConnectorID, c.ArtifactSHA256})
	return b
}

func validPreparing(data []byte) bool {
	var r preparingRecord
	if len(data) > 1024 || json.Unmarshal(data, &r) != nil || r.Version != "observer-connected-preparing/v1" || r.State != "PREPARING" || !connectedprofile.ValidServerID(r.ServerID) || len(r.ManifestSHA256) != 64 {
		return false
	}
	for _, ch := range r.ManifestSHA256 {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	canonical, _ := json.Marshal(r)
	return bytes.Equal(data, canonical)
}

func inspectInstalled() (connectedprofile.Config, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return connectedprofile.Config{}, ErrUnsafe
	}
	c, err := connectedprofile.Load()
	if err != nil {
		return c, ErrUnsafe
	}
	release := connectedprofile.ReleaseDirectory + "/" + c.ArtifactSHA256
	d, err := rootDirectory(release)
	if err != nil {
		return c, ErrUnsafe
	}
	defer d.Close()
	for _, name := range append(connectedbundle.Names(), connectedbundle.ManifestName) {
		fd, err := unix.Openat(int(d.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err != nil {
			return c, ErrUnsafe
		}
		f := os.NewFile(uintptr(fd), name)
		mode := os.FileMode(0644)
		if strings.HasPrefix(name, "observer-connected-") {
			mode = 0755
		}
		valid := regular(f, 0, mode, 200<<20)
		f.Close()
		if !valid {
			return c, ErrUnsafe
		}
	}
	if _, err := connectedbundle.VerifyDirectory(release, c.ArtifactSHA256); err != nil {
		return c, ErrUnsafe
	}
	units, err := connectedunits.RenderUnits(c)
	if err != nil {
		return c, ErrUnsafe
	}
	for name, want := range map[string][]byte{collectorUnit: units.Collector, uploaderUnit: units.Uploader} {
		actual, err := connectedprofile.ReadRootFile("/etc/systemd/system/"+name, connectedunits.MaxRenderedBytes)
		if err != nil || !bytes.Equal(actual, want) {
			return c, ErrUnsafe
		}
	}
	return c, nil
}

func ReadStatus() (Status, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return Status{}, ErrUnsafe
	}
	if _, err := os.Lstat(connectedprofile.ConfigPath); os.IsNotExist(err) {
		data, e := connectedprofile.ReadRootFile(connectedprofile.ConfigDirectory+"/preparing.json", 1024)
		if e == nil && validPreparing(data) {
			return Status{State: "PREPARING"}, nil
		}
		return Status{}, ErrUnsafe
	}
	if c, err := connectedprofile.Load(); err == nil {
		for _, entry := range []struct{ name, state string }{{"uninstalled.json", "UNINSTALLED_LOCAL_STATE_RETAINED"}, {"uninstalling.json", "UNINSTALL_RECOVERY_REQUIRED"}} {
			path := connectedprofile.ConfigDirectory + "/" + entry.name
			if _, e := os.Lstat(path); os.IsNotExist(e) {
				continue
			} else if e != nil {
				return Status{}, ErrUnsafe
			}
			data, e := connectedprofile.ReadRootFile(path, 1024)
			if e != nil || !bytes.Equal(data, removalBytes(c, entry.state)) {
				return Status{}, ErrUnsafe
			}
			return Status{State: entry.state}, nil
		}
	}
	c, err := inspectInstalled()
	if err != nil {
		return Status{}, ErrUnsafe
	}
	result := Status{State: "INSTALLED_READY"}
	collector, err := readStatusRecord("collector-status", c.CollectorUID)
	if err == nil {
		result.Collector = &collector
	} else if !os.IsNotExist(err) {
		return Status{}, ErrUnsafe
	}
	uploader, err := readStatusRecord("uploader-status", c.UploaderUID)
	if err == nil {
		if uploader.State == "ACKNOWLEDGED_FRESH" && (uploader.CollectedAt.IsZero() || time.Since(uploader.CollectedAt) > connectedstatus.FreshnessWindow || uploader.CollectedAt.After(time.Now().Add(uploadstate.MaxFuture))) {
			uploader.State = "ACKNOWLEDGED_STALE"
		}
		result.Uploader = &uploader
	} else if !os.IsNotExist(err) {
		return Status{}, ErrUnsafe
	}
	return result, nil
}

func readStatusRecord(name string, uid uint32) (connectedstatus.Record, error) {
	parent, err := rootDirectory(connectedprofile.StateDirectory)
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	defer parent.Close()
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	dir := os.NewFile(uintptr(fd), name)
	defer dir.Close()
	i, err := dir.Stat()
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uid || i.Mode().Perm() != 0700 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(fd) {
		return connectedstatus.Record{}, ErrUnsafe
	}
	next, err := unix.Openat(fd, "status.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return connectedstatus.Record{}, os.ErrNotExist
	}
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	f := os.NewFile(uintptr(next), "status")
	defer f.Close()
	if !regular(f, uid, 0600, 1024) {
		return connectedstatus.Record{}, ErrUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return connectedstatus.Record{}, ErrUnsafe
	}
	return connectedstatus.Decode(data)
}

// Stop holds the same installation lease as enrollment and never targets an
// unknown unit. Existing immutable artifacts and exact unit bytes must match.
func Stop(ctx context.Context) error {
	if ctx == nil {
		return ErrUnsafe
	}
	lock, err := acquireLease()
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := inspectInstalled(); err != nil {
		return err
	}
	return stopOwnedWorkers(ctx)
}

func stopOwnedWorkers(ctx context.Context) error {
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if verifyManagedUnit(ctx, unit) != nil {
			return ErrUnsafe
		}
	}
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if service(ctx, "stop", unit) != nil {
			return ErrRecovery
		}
		out, err := command(ctx, "/usr/bin/systemctl", []string{"show", "--property=ActiveState", "--property=MainPID", "--", unit}, nil, 256)
		if err != nil || !stoppedProperties(out) {
			return ErrRecovery
		}
	}
	return nil
}

func stoppedProperties(data []byte) bool {
	return string(data) == "ActiveState=inactive\nMainPID=0\n" || string(data) == "MainPID=0\nActiveState=inactive\n"
}

// Uninstall detaches only exact owned services. Credentials, ledger, accounts,
// immutable binaries and diagnostics remain for explicit recovery/revocation.
// It does not claim to revoke a remote registration or recursively delete state.
func Uninstall(ctx context.Context) error {
	if ctx == nil {
		return ErrUnsafe
	}
	lock, err := acquireLease()
	if err != nil {
		return err
	}
	defer lock.Close()
	c, err := inspectInstalled()
	if err != nil {
		return err
	}
	if err := stopOwnedWorkers(ctx); err != nil {
		return err
	}
	configRoot, err := os.OpenRoot(connectedprofile.ConfigDirectory)
	if err != nil {
		return ErrRecovery
	}
	defer configRoot.Close()
	if publishNew(configRoot, "uninstalling.json", removalBytes(c, "UNINSTALL_RECOVERY_REQUIRED"), 0600) != nil {
		return ErrRecovery
	}
	quarantine := connectedprofile.ConfigDirectory + "/disabled-units"
	if mkdirNew(quarantine, 0, 0, 0700) != nil {
		return ErrRecovery
	}
	destination, err := rootDirectory(quarantine)
	if err != nil {
		return ErrRecovery
	}
	defer destination.Close()
	source, err := rootDirectory("/etc/systemd/system")
	if err != nil {
		return ErrRecovery
	}
	defer source.Close()
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if verifyManagedUnit(ctx, unit) != nil || service(ctx, "disable", unit) != nil {
			return ErrRecovery
		}
		if unix.Renameat2(int(source.Fd()), unit, int(destination.Fd()), unit, unix.RENAME_NOREPLACE) != nil || destination.Sync() != nil || source.Sync() != nil {
			return ErrRecovery
		}
	}
	if _, err := command(ctx, "/usr/bin/systemctl", []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if publishNew(configRoot, "uninstalled.json", removalBytes(c, "UNINSTALLED_LOCAL_STATE_RETAINED"), 0600) != nil {
		return ErrRecovery
	}
	return nil
}

func verifyManagedUnit(ctx context.Context, unit string) error {
	if unit != uploaderUnit && unit != collectorUnit {
		return ErrUnsafe
	}
	out, err := command(ctx, "/usr/bin/systemctl", []string{"show", "--property=Id", "--property=FragmentPath", "--property=DropInPaths", "--property=NeedDaemonReload", "--property=LoadState", "--", unit}, nil, 2048)
	if err != nil {
		return ErrUnsafe
	}
	want := map[string]string{"Id": unit, "FragmentPath": "/etc/systemd/system/" + unit, "DropInPaths": "", "NeedDaemonReload": "no", "LoadState": "loaded"}
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || want[k] != v {
			return ErrUnsafe
		}
		if _, ok := want[k]; !ok {
			return ErrUnsafe
		}
		delete(want, k)
	}
	if len(want) != 0 {
		return ErrUnsafe
	}
	return nil
}
