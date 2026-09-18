//go:build linux

package connectedinstall

import (
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

// Installation destinations are closed, code-owned roles. The only variable
// path component is the already-verified canonical bundle digest.
type installLayout struct {
	digest                                 string
	collectorUID, uploaderUID, uploaderGID uint32
	sharedGID                              uint32
}

type directoryRole uint8

const (
	dirOptRoot directoryRole = iota + 1
	dirReleases
	dirRelease
	dirHandoff
	dirCollectorStatus
	dirUploaderStatus
	dirEnrollment
	dirUploaderRoot
	dirUploaderBin
	dirUploaderEtc
	dirUploaderSSL
	dirUploaderCerts
	dirUploaderConfig
	dirUploaderState
	dirUploaderStateEnrollment
	dirUploaderStateLedger
	dirUploaderStateStatus
	dirUploaderHandoff
	dirUploaderActivation
	dirCredentials
)

type fileRole uint8

const (
	fileCollectorUnit fileRole = iota + 1
	fileUploaderUnit
	fileInstalledConfig
	fileCredential
	fileCA
	fileHosts
	fileResolver
	fileNSS
)

type directorySpec struct {
	parent, name string
	uid, gid     uint32
	mode         os.FileMode
}

type fileSpec struct {
	parent, name string
	mode         os.FileMode
	limit        int
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (l installLayout) directory(role directoryRole) (directorySpec, error) {
	root := connectedprofile.StateDirectory
	uploader := root + "/uploader-root"
	rootSpec := func(parent, name string, mode os.FileMode) directorySpec {
		return directorySpec{parent: parent, name: name, mode: mode}
	}
	switch role {
	case dirOptRoot:
		return rootSpec("/opt", "home-lab-observer-connected", 0755), nil
	case dirReleases:
		return rootSpec("/opt/home-lab-observer-connected", "releases", 0755), nil
	case dirRelease:
		if !digestPattern.MatchString(l.digest) {
			return directorySpec{}, ErrUnsafe
		}
		return rootSpec(connectedprofile.ReleaseDirectory, l.digest, 0755), nil
	case dirHandoff:
		return directorySpec{root, "handoff", l.collectorUID, l.sharedGID, 0750}, l.requirePrincipals()
	case dirCollectorStatus:
		return directorySpec{root, "collector-status", l.collectorUID, l.sharedGID, 0700}, l.requirePrincipals()
	case dirUploaderStatus:
		return directorySpec{root, "uploader-status", l.uploaderUID, l.uploaderGID, 0700}, l.requirePrincipals()
	case dirEnrollment:
		return directorySpec{root, "enrollment", l.uploaderUID, l.uploaderGID, 0700}, l.requirePrincipals()
	case dirUploaderRoot:
		return rootSpec(root, "uploader-root", 0755), nil
	case dirUploaderBin:
		return rootSpec(uploader, "bin", 0755), nil
	case dirUploaderEtc:
		return rootSpec(uploader, "etc", 0755), nil
	case dirUploaderSSL:
		return rootSpec(uploader+"/etc", "ssl", 0755), nil
	case dirUploaderCerts:
		return rootSpec(uploader+"/etc/ssl", "certs", 0755), nil
	case dirUploaderConfig:
		return rootSpec(uploader+"/etc", "home-lab-observer-connected", 0755), nil
	case dirUploaderState:
		return rootSpec(uploader, "state", 0755), nil
	case dirUploaderStateEnrollment:
		return rootSpec(uploader+"/state", "enrollment", 0755), nil
	case dirUploaderStateLedger:
		return rootSpec(uploader+"/state", "ledger", 0755), nil
	case dirUploaderStateStatus:
		return rootSpec(uploader+"/state", "status", 0755), nil
	case dirUploaderHandoff:
		return rootSpec(uploader, "handoff", 0755), nil
	case dirUploaderActivation:
		return rootSpec(uploader, "activation", 0755), nil
	case dirCredentials:
		return rootSpec(connectedprofile.ConfigDirectory, "credentials", 0700), nil
	default:
		return directorySpec{}, ErrUnsafe
	}
}

func (l installLayout) requirePrincipals() error {
	if l.collectorUID == 0 || l.uploaderUID == 0 || l.uploaderGID == 0 || l.sharedGID == 0 ||
		l.collectorUID == l.uploaderUID || l.uploaderGID == l.sharedGID {
		return ErrUnsafe
	}
	return nil
}

func (l installLayout) file(role fileRole) (fileSpec, error) {
	uploaderEtc := connectedprofile.StateDirectory + "/uploader-root/etc"
	switch role {
	case fileCollectorUnit:
		return fileSpec{"/etc/systemd/system", collectorUnit, 0644, connectedunits.MaxRenderedBytes}, nil
	case fileUploaderUnit:
		return fileSpec{"/etc/systemd/system", uploaderUnit, 0644, connectedunits.MaxRenderedBytes}, nil
	case fileInstalledConfig:
		return fileSpec{connectedprofile.ConfigDirectory, "installed.json", 0644, connectedprofile.MaxConfigBytes}, nil
	case fileCredential:
		return fileSpec{connectedprofile.ConfigDirectory + "/credentials", "connector.json", 0600, 512}, nil
	case fileCA:
		return fileSpec{uploaderEtc + "/ssl/certs", "ca-certificates.crt", 0644, 1 << 20}, nil
	case fileHosts:
		return fileSpec{uploaderEtc, "hosts", 0644, 4096}, nil
	case fileResolver:
		return fileSpec{uploaderEtc, "resolv.conf", 0644, 4096}, nil
	case fileNSS:
		return fileSpec{uploaderEtc, "nsswitch.conf", 0644, 4096}, nil
	default:
		return fileSpec{}, ErrUnsafe
	}
}

func (l installLayout) path(role directoryRole) (string, error) {
	spec, err := l.directory(role)
	if err != nil {
		return "", err
	}
	return filepath.Join(spec.parent, spec.name), nil
}

// createDirectory refuses all existing entries. A failed operation preserves
// its newly created role for explicit recovery, never automatic adoption.
func (l installLayout) createDirectory(role directoryRole) error {
	spec, err := l.directory(role)
	if err != nil {
		return ErrUnsafe
	}
	parent, err := rootDirectory(spec.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	return createDirectoryAt(parent, spec, nil)
}

// createDirectoryAt is a synthetic-root fault boundary. Production selects
// its fixed role and validates every ancestor before reaching this function.
func createDirectoryAt(parent *os.File, spec directorySpec, syncFile func(*os.File) error) error {
	if !trustedRootParent(parent) || spec.name == "" || spec.mode == 0 {
		return ErrUnsafe
	}
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	if unix.Mkdirat(int(parent.Fd()), spec.name, 0700) != nil {
		return ErrRecovery
	}
	fd, err := unix.Openat2(int(parent.Fd()), spec.name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return ErrRecovery
	}
	directory := os.NewFile(uintptr(fd), spec.name)
	defer directory.Close()
	if directory.Chown(int(spec.uid), int(spec.gid)) != nil || directory.Chmod(spec.mode) != nil ||
		!exactDirectory(directory, spec) || syncFile(directory) != nil || syncFile(parent) != nil {
		return ErrRecovery
	}
	return nil
}

func trustedRootParent(parent *os.File) bool {
	if parent == nil {
		return false
	}
	i, err := parent.Stat()
	if err != nil || !i.IsDir() || i.Mode().Perm()&0022 != 0 ||
		i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(int(parent.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
}

func exactDirectory(directory *os.File, spec directorySpec) bool {
	i, err := directory.Stat()
	if err != nil || !i.IsDir() || i.Mode().Perm() != spec.mode ||
		i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(int(directory.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == spec.uid && s.Gid == spec.gid
}

// publishFile has no caller-selected destination. Each role gets its own
// temporary name; interrupted writes remain distinct recovery evidence.
func (l installLayout) publishFile(role fileRole, data []byte) error {
	spec, err := l.file(role)
	if err != nil || len(data) == 0 || len(data) > spec.limit {
		return ErrUnsafe
	}
	parent, err := rootDirectory(spec.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	return publishFileAt(parent, spec, data, nil)
}

// publishFileAt is a synthetic-root fault boundary for one closed file role.
func publishFileAt(parent *os.File, spec fileSpec, data []byte, syncFile func(*os.File) error) error {
	if !trustedRootParent(parent) || spec.name == "" || spec.limit < 1 || len(data) == 0 || len(data) > spec.limit {
		return ErrUnsafe
	}
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	var existing unix.Stat_t
	if unix.Fstatat(int(parent.Fd()), spec.name, &existing, unix.AT_SYMLINK_NOFOLLOW) != unix.ENOENT {
		return ErrRecovery
	}
	temp := "." + spec.name + ".install-next"
	fd, err := unix.Openat2(int(parent.Fd()), temp, &unix.OpenHow{
		Flags:   unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC,
		Mode:    uint64(spec.mode),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return ErrRecovery
	}
	f := os.NewFile(uintptr(fd), temp)
	if f.Chown(0, 0) != nil || f.Chmod(spec.mode) != nil || !regular(f, 0, spec.mode, 0) {
		f.Close()
		return ErrRecovery
	}
	n, writeErr := f.Write(data)
	syncErr := syncFile(f)
	closeErr := f.Close()
	if writeErr != nil || n != len(data) || syncErr != nil || closeErr != nil || syncFile(parent) != nil {
		return ErrRecovery
	}
	if unix.Renameat2(int(parent.Fd()), temp, int(parent.Fd()), spec.name, unix.RENAME_NOREPLACE) != nil || syncFile(parent) != nil {
		return ErrRecovery
	}
	return nil
}
