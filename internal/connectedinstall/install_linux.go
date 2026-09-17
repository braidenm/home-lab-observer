//go:build linux

package connectedinstall

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

const collectorName = "hlo-connected-collector"
const uploaderName = "hlo-connected-uploader"
const sharedName = "hlo-connected-read"

type Request struct {
	BundleDirectory, ManifestSHA256, ServerID string
	Grant                                     []byte
}

var grantPattern = regexp.MustCompile(`^hle_[A-Za-z0-9_-]{43}$`)

// CheckRequest performs only non-mutating local prerequisites before prompting.
// Install repeats validation under its lease; this result is not an authority token.
func CheckRequest(ctx context.Context, bundle, digest, server string) error {
	if ctx == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !connectedprofile.ValidServerID(server) {
		return ErrUnsafe
	}
	if _, err := connectedbundle.VerifyDirectory(bundle, digest); err != nil {
		return ErrUnsafe
	}
	return preflight(ctx)
}

// Install creates a separate canary only. It never adopts unknown resources,
// resets a ledger, alters the existing connector, or repairs partial setup.
func Install(ctx context.Context, request Request) (result error) {
	phase := "PREFLIGHT_REFUSED"
	defer func() {
		if result != nil {
			result = &PhaseError{Phase: phase}
		}
	}()
	if ctx == nil || ctx.Err() != nil || !connectedprofile.ValidServerID(request.ServerID) || len(request.Grant) != 47 || !grantPattern.Match(request.Grant) {
		return ErrUnsafe
	}
	manifest, err := connectedbundle.VerifyDirectory(request.BundleDirectory, request.ManifestSHA256)
	if err != nil {
		return ErrUnsafe
	}
	lock, err := acquireLease()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := preflight(ctx); err != nil {
		return err
	}
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil {
		return err
	}
	if connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	for _, path := range []string{connectedprofile.ConfigDirectory, connectedprofile.StateDirectory, "/opt/home-lab-observer-connected", "/etc/systemd/system/" + collectorUnit, "/etc/systemd/system/" + uploaderUnit} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	if _, err := user.Lookup(collectorName); !missingUser(err) {
		return ErrUnsafe
	}
	if _, err := user.Lookup(uploaderName); !missingUser(err) {
		return ErrUnsafe
	}
	if _, err := user.LookupGroup(sharedName); !missingGroup(err) {
		return ErrUnsafe
	}
	if _, err := user.LookupGroup(uploaderName); !missingGroup(err) {
		return ErrUnsafe
	}
	phase = "LOCAL_SETUP_INCOMPLETE"
	if mkdirNew(connectedprofile.ConfigDirectory, 0, 0, 0755) != nil {
		return ErrRecovery
	}
	configRoot, err := os.OpenRoot(connectedprofile.ConfigDirectory)
	if err != nil {
		return ErrRecovery
	}
	defer configRoot.Close()
	preparing, _ := json.Marshal(preparingRecord{Version: "observer-connected-preparing/v1", State: "PREPARING", ServerID: request.ServerID, ManifestSHA256: request.ManifestSHA256})
	if publishNew(configRoot, "preparing.json", preparing, 0600) != nil {
		return ErrRecovery
	}
	for _, path := range []string{connectedprofile.StateDirectory, "/opt/home-lab-observer-connected", connectedprofile.ReleaseDirectory} {
		if mkdirNew(path, 0, 0, 0755) != nil {
			return ErrRecovery
		}
	}
	for _, name := range []string{sharedName, uploaderName} {
		if _, err := command(ctx, "/usr/sbin/groupadd", []string{"--system", name}, nil, 1024); err != nil {
			return ErrRecovery
		}
	}
	if _, err := command(ctx, "/usr/sbin/useradd", []string{"--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "--gid", sharedName, collectorName}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if _, err := command(ctx, "/usr/sbin/useradd", []string{"--system", "--no-create-home", "--shell", "/usr/sbin/nologin", "--gid", uploaderName, "--groups", sharedName, uploaderName}, nil, 1024); err != nil {
		return ErrRecovery
	}
	collector, err := user.Lookup(collectorName)
	if err != nil {
		return ErrRecovery
	}
	uploader, err := user.Lookup(uploaderName)
	if err != nil {
		return ErrRecovery
	}
	shared, err := user.LookupGroup(sharedName)
	if err != nil {
		return ErrRecovery
	}
	cu, e1 := strconv.ParseUint(collector.Uid, 10, 32)
	uu, e2 := strconv.ParseUint(uploader.Uid, 10, 32)
	ug, e3 := strconv.ParseUint(uploader.Gid, 10, 32)
	sg, e4 := strconv.ParseUint(shared.Gid, 10, 32)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return ErrRecovery
	}
	policy := connectedunits.EnrollmentInput{UploaderUID: uint32(uu), UploaderGID: uint32(ug), SharedGID: uint32(sg), ArtifactSHA256: request.ManifestSHA256, Addresses: addresses}
	release := connectedprofile.ReleaseDirectory + "/" + request.ManifestSHA256
	if mkdirNew(release, 0, 0, 0755) != nil {
		return ErrRecovery
	}
	if err := copyBundle(request.BundleDirectory, release, manifest, request.ManifestSHA256); err != nil {
		return err
	}
	if err := prepareRoot(release, uint32(cu), uint32(uu), uint32(ug), uint32(sg), addresses); err != nil {
		return err
	}
	invocation, err := connectedunits.RenderEnrollmentProperties(policy, "enroll")
	if err != nil {
		return ErrUnsafe
	}
	grantData, _ := json.Marshal(connectedenroll.GrantInput{ServerID: request.ServerID, Grant: string(request.Grant), UploaderUID: uint32(uu), UploaderGID: uint32(ug), SharedGID: uint32(sg)})
	clear(request.Grant)
	phase = "ENROLLMENT_RECOVERY_REQUIRED"
	out, err := runEnrollment(ctx, invocation.Properties, invocation.Executable, "enroll", grantData, policy)
	clear(grantData)
	if err != nil {
		if err == ErrUnsafe {
			phase = "LOCAL_SETUP_INCOMPLETE"
		}
		return ErrRecovery
	}
	var binding connectedenroll.Binding
	if json.Unmarshal(out, &binding) != nil || binding.ServerID != request.ServerID || !connectedprofile.ValidConnectorID(binding.ConnectorID) {
		return ErrRecovery
	}
	canonical, _ := json.Marshal(binding)
	if string(canonical) != string(out) {
		return ErrRecovery
	}
	config := connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: binding.ServerID, ConnectorID: binding.ConnectorID, CollectorUID: uint32(cu), UploaderUID: uint32(uu), UploaderGID: uint32(ug), SharedGID: uint32(sg), ArtifactSHA256: request.ManifestSHA256, PolicyGeneration: 1, Addresses: addresses}
	if config.Validate() != nil {
		return ErrRecovery
	}
	validation, _ := json.Marshal(connectedenroll.ValidationInput{ServerID: binding.ServerID, ConnectorID: binding.ConnectorID, UploaderUID: uint32(uu), UploaderGID: uint32(ug), SharedGID: uint32(sg)})
	invocation, err = connectedunits.RenderEnrollmentProperties(policy, "validate-enrollment")
	if err != nil {
		return ErrRecovery
	}
	secret, err := runEnrollment(ctx, invocation.Properties, invocation.Executable, "validate-enrollment", validation, policy)
	defer clear(secret)
	if err != nil {
		return ErrRecovery
	}
	if _, err := connectedcredential.DecodeCredential(secret, binding.ServerID, binding.ConnectorID); err != nil {
		return ErrRecovery
	}
	if err := promote(secret, uint32(uu), uint32(ug)); err != nil {
		return err
	}
	invocation, err = connectedunits.RenderEnrollmentProperties(policy, "validate-ledger")
	if err != nil {
		return ErrRecovery
	}
	validationResult, err := runEnrollment(ctx, invocation.Properties, invocation.Executable, "validate-ledger", validation, policy)
	if err != nil || string(validationResult) != "VALID" {
		return ErrRecovery
	}
	units, err := connectedunits.RenderUnits(config)
	if err != nil {
		return ErrRecovery
	}
	unitRoot, err := os.OpenRoot("/etc/systemd/system")
	if err != nil {
		return ErrRecovery
	}
	defer unitRoot.Close()
	if publishNew(unitRoot, collectorUnit, units.Collector, 0644) != nil || publishNew(unitRoot, uploaderUnit, units.Uploader, 0644) != nil {
		return ErrRecovery
	}
	if _, err := command(ctx, "/usr/bin/systemctl", []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if connectedpolicy.ValidateEffective(ctx, uploaderUnit, addresses) != nil {
		return ErrRecovery
	}
	data, _ := connectedprofile.Encode(config)
	if publishNew(configRoot, "installed.json", data, 0644) != nil {
		return ErrRecovery
	}
	// Activation is deliberately separate from durable installation. The installed
	// acceptance gate must authorize the exact profile before start is exposed.
	return nil
}

func preflight(ctx context.Context) error {
	if runtime.GOARCH != "amd64" {
		return ErrUnsafe
	}
	for _, path := range []string{"/etc", "/var/lib", "/opt", "/etc/systemd/system"} {
		f, err := rootDirectory(path)
		if err != nil {
			return ErrUnsafe
		}
		if f.Close() != nil {
			return ErrUnsafe
		}
	}
	osRelease, err := connectedprofile.ReadRootFile("/usr/lib/os-release", 4096)
	if err != nil || !ubuntu2404(osRelease) {
		return ErrUnsafe
	}
	out, err := command(ctx, "/usr/bin/systemctl", []string{"--version"}, nil, 4096)
	version := strings.Fields(string(out))
	if err != nil || len(version) < 2 || version[0] != "systemd" || version[1] != "255" {
		return ErrUnsafe
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		return ErrUnsafe
	}
	return nil
}

func copyBundle(source, destination string, manifest connectedbundle.Manifest, digest string) error {
	root, err := os.OpenRoot(destination)
	if err != nil {
		return ErrRecovery
	}
	defer root.Close()
	input, err := os.OpenRoot(source)
	if err != nil {
		return ErrUnsafe
	}
	defer input.Close()
	inputDirectory, err := input.Open(".")
	if err != nil {
		return ErrUnsafe
	}
	defer inputDirectory.Close()
	for _, name := range append(connectedbundle.Names(), connectedbundle.ManifestName) {
		mode := os.FileMode(0644)
		if strings.HasPrefix(name, "observer-connected-") {
			mode = 0755
		}
		fd, err := unix.Openat(int(inputDirectory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return ErrRecovery
		}
		in := os.NewFile(uintptr(fd), name)
		info, err := in.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 200<<20 {
			in.Close()
			return ErrRecovery
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Nlink != 1 {
			in.Close()
			return ErrRecovery
		}
		out, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			in.Close()
			return ErrRecovery
		}
		if out.Chmod(mode) != nil || !regular(out, 0, mode, 0) {
			in.Close()
			out.Close()
			return ErrRecovery
		}
		_, copyErr := io.Copy(out, io.LimitReader(in, 201<<20))
		closeIn := in.Close()
		syncErr := out.Sync()
		closeOut := out.Close()
		if copyErr != nil || closeIn != nil || syncErr != nil || closeOut != nil {
			return ErrRecovery
		}
	}
	_ = manifest // VerifyDirectory below independently rechecks every manifest entry.
	if _, err := connectedbundle.VerifyDirectory(destination, digest); err != nil {
		return ErrRecovery
	}
	d, err := root.Open(".")
	if err != nil {
		return ErrRecovery
	}
	defer d.Close()
	if d.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func prepareRoot(release string, collector, uploader, primary, shared uint32, addresses []string) error {
	state := connectedprofile.StateDirectory
	for _, item := range []struct {
		name     string
		uid, gid uint32
		mode     os.FileMode
	}{{"handoff", collector, shared, 0750}, {"collector-status", collector, shared, 0700}, {"uploader-status", uploader, primary, 0700}, {"enrollment", uploader, primary, 0700}, {"uploader-root", 0, 0, 0755}} {
		if mkdirNew(state+"/"+item.name, int(item.uid), int(item.gid), item.mode) != nil {
			return ErrRecovery
		}
	}
	base := state + "/uploader-root"
	for _, name := range []string{"bin", "etc", "etc/ssl", "etc/ssl/certs", "etc/home-lab-observer-connected", "state", "state/enrollment", "state/ledger", "state/status", "handoff", "activation"} {
		if mkdirNew(base+"/"+name, 0, 0, 0755) != nil {
			return ErrRecovery
		}
	}
	// Copy only reviewed CA bytes; never mount the host /etc or a shell into root.
	ca, err := connectedprofile.ReadRootFile("/etc/ssl/certs/ca-certificates.crt", 1024*1024)
	if err != nil {
		return ErrRecovery
	}
	_ = release
	files := map[string][]byte{"etc/ssl/certs/ca-certificates.crt": ca, "etc/resolv.conf": []byte("# No runtime DNS resolver\n"), "etc/nsswitch.conf": []byte("hosts: files\n")}
	var hosts strings.Builder
	for _, a := range addresses {
		hosts.WriteString(a + " " + connectedprofile.Hostname + "\n")
	}
	files["etc/hosts"] = []byte(hosts.String())
	for name, data := range files {
		parent, err := os.OpenRoot(filepath.Dir(base + "/" + name))
		if err != nil {
			return ErrRecovery
		}
		if name == "etc/ssl/certs/ca-certificates.crt" {
			err = publishCA(parent, data)
		} else {
			err = publishNew(parent, filepath.Base(name), data, 0644)
		}
		parent.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

type PhaseError struct{ Phase string }

func (e *PhaseError) Error() string { return e.Phase }

func ubuntu2404(data []byte) bool {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || (key != "ID" && key != "VERSION_ID") {
			continue
		}
		if _, exists := values[key]; exists {
			return false
		}
		values[key] = strings.Trim(value, `"`)
	}
	return values["ID"] == "ubuntu" && values["VERSION_ID"] == "24.04"
}

func missingUser(err error) bool {
	var missing user.UnknownUserError
	return errors.As(err, &missing)
}

func missingGroup(err error) bool {
	var missing user.UnknownGroupError
	return errors.As(err, &missing)
}

func promote(secret []byte, uid, gid uint32) error {
	parent, err := rootDirectory(connectedprofile.StateDirectory)
	if err != nil {
		return ErrRecovery
	}
	defer parent.Close()
	stage, err := privateDirectoryAt(parent, "enrollment", uid, gid)
	if err != nil {
		return ErrRecovery
	}
	defer stage.Close()
	ledger, err := privateDirectoryAt(stage, "ledger", uid, gid)
	if err != nil {
		return ErrRecovery
	}
	defer ledger.Close()
	before, err := ledger.Stat()
	if err != nil {
		return ErrRecovery
	}
	if stage.Chown(0, 0) != nil || stage.Chmod(0700) != nil || stage.Sync() != nil {
		return ErrRecovery
	}
	credentialDirectory := connectedprofile.ConfigDirectory + "/credentials"
	if mkdirNew(credentialDirectory, 0, 0, 0700) != nil {
		return ErrRecovery
	}
	credentials, err := os.OpenRoot(credentialDirectory)
	if err != nil {
		return ErrRecovery
	}
	defer credentials.Close()
	if publishNew(credentials, "connector.json", secret, 0600) != nil {
		return ErrRecovery
	}
	if unix.Renameat2(int(stage.Fd()), "ledger", int(parent.Fd()), "ledger", unix.RENAME_NOREPLACE) != nil || stage.Sync() != nil || parent.Sync() != nil {
		return ErrRecovery
	}
	promoted, err := privateDirectoryAt(parent, "ledger", uid, gid)
	if err != nil {
		return ErrRecovery
	}
	defer promoted.Close()
	after, err := promoted.Stat()
	if err != nil || !os.SameFile(before, after) {
		return ErrRecovery
	}
	return nil
}

func privateDirectoryAt(parent *os.File, name string, uid, gid uint32) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != uid || st.Gid != gid || st.Mode&07777 != 0700 || !noACL(fd) {
		f.Close()
		return nil, ErrUnsafe
	}
	return f, nil
}
