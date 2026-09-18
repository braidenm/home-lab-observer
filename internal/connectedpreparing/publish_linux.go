//go:build linux

// Package connectedpreparing publishes only the fixed first-install marker.
// It does not acquire the installation lease or authorize installer progress.
package connectedpreparing

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const markerName = "preparing.json"
const nextName = ".preparing-next"
const maxMarkerBytes = 1024

var ErrUnsafe = errors.New("connected_preparing_unsafe")
var ErrRecovery = errors.New("connected_preparing_recovery_required")

type Record struct {
	ServerID       string
	ManifestSHA256 string
}

type marker struct {
	Version        string `json:"version"`
	State          string `json:"state"`
	ServerID       string `json:"server_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

func (r Record) encode() ([]byte, error) {
	if !connectedprofile.ValidServerID(r.ServerID) || len(r.ManifestSHA256) != 64 {
		return nil, ErrUnsafe
	}
	for _, ch := range r.ManifestSHA256 {
		if ch < '0' || ch > '9' {
			if ch < 'a' || ch > 'f' {
				return nil, ErrUnsafe
			}
		}
	}
	data, err := json.Marshal(marker{"observer-connected-preparing/v1", "PREPARING", r.ServerID, r.ManifestSHA256})
	if err != nil || len(data) == 0 || len(data) > maxMarkerBytes {
		return nil, ErrUnsafe
	}
	return data, nil
}

// Publish reaches only the fixed config directory. A future installer must
// independently hold the lease and prove an entirely new installation.
func Publish(record Record) error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	etc, err := openTrustedEtc()
	if err != nil {
		return err
	}
	defer etc.Close()
	config, err := openDirAt(int(etc.Fd()), "home-lab-observer-connected")
	if err != nil {
		return ErrUnsafe
	}
	defer config.Close()
	return publishAt(config, record, nil)
}

func openTrustedEtc() (*os.File, error) {
	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer unix.Close(rootFD)
	etc, err := openDirAt(rootFD, "etc")
	if err != nil {
		return nil, ErrUnsafe
	}
	if !trustedDirectory(etc, false) {
		etc.Close()
		return nil, ErrUnsafe
	}
	return etc, nil
}

func openDirAt(parent int, name string) (*os.File, error) {
	fd, err := unix.Openat2(parent, name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

// publishAt is a synthetic-fixture seam, not an arbitrary public writer.
// Interruption leaves the fixed temporary role for explicit recovery.
func publishAt(config *os.File, record Record, afterSync func() error) error {
	if os.Getuid() != 0 || os.Geteuid() != 0 || config == nil || !trustedDirectory(config, true) {
		return ErrUnsafe
	}
	data, err := record.encode()
	if err != nil {
		return err
	}
	listing, err := openDirAt(int(config.Fd()), ".")
	if err != nil {
		return ErrUnsafe
	}
	names, readErr := listing.Readdirnames(1)
	closeErr := listing.Close()
	if closeErr != nil || len(names) != 0 || readErr != io.EOF {
		return ErrRecovery
	}
	fd, err := unix.Openat2(int(config.Fd()), nextName, &unix.OpenHow{
		Flags:   unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC,
		Mode:    0600,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return ErrRecovery
	}
	file := os.NewFile(uintptr(fd), nextName)
	if !trustedMarker(file, 0) {
		file.Close()
		return ErrRecovery
	}
	n, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr = file.Close()
	if n != len(data) || writeErr != nil || syncErr != nil || closeErr != nil || config.Sync() != nil {
		return ErrRecovery
	}
	if afterSync != nil && afterSync() != nil {
		return ErrRecovery
	}
	if unix.Renameat2(int(config.Fd()), nextName, int(config.Fd()), markerName, unix.RENAME_NOREPLACE) != nil || config.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func trustedDirectory(dir *os.File, exactMode bool) bool {
	i, err := dir.Stat()
	if err != nil || !i.IsDir() || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 ||
		i.Mode().Perm()&0022 != 0 || exactMode && i.Mode().Perm() != 0755 || !noACL(int(dir.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
}

func trustedMarker(file *os.File, size int64) bool {
	i, err := file.Stat()
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 ||
		i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() != size || !noACL(int(file.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0 && s.Nlink == 1
}

func noACL(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Fgetxattr(fd, name, nil)
		if err == unix.ENODATA || err == unix.ENOTSUP {
			continue
		}
		if err != nil || n != 0 {
			return false
		}
	}
	return true
}
