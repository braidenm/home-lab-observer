//go:build linux

package connectedcredential

import (
	"encoding/binary"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const directory = "/run/credentials/home-lab-observer-connected-uploader.service"

// Load accepts only the actual systemd255 RootDirectory credential layout
// independently probed with a synthetic credential, including its per-UID ACL.
// The final packaged unit remains subject to its own installed acceptance gate.
func Load(server, connector string) (CredentialRecord, error) {
	if os.Geteuid() == 0 || os.Getenv("CREDENTIALS_DIRECTORY") != directory {
		return CredentialRecord{}, ErrUnsafe
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return CredentialRecord{}, ErrUnsafe
	}
	defer func() { unix.Close(fd) }()
	parts := []string{"run", "credentials", "home-lab-observer-connected-uploader.service", "connector.json"}
	for i, name := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if i < 3 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, name, flags, 0)
		if e != nil {
			return CredentialRecord{}, ErrUnsafe
		}
		unix.Close(fd)
		fd = next
		var s unix.Stat_t
		if unix.Fstat(fd, &s) != nil || s.Uid != 0 || s.Gid != 0 {
			return CredentialRecord{}, ErrUnsafe
		}
		switch i {
		case 0:
			if s.Mode&07777 != 0755 && s.Mode&07777 != 01777 {
				return CredentialRecord{}, ErrUnsafe
			}
		case 1:
			if s.Mode&07777 != 0755 {
				return CredentialRecord{}, ErrUnsafe
			}
		case 2:
			if s.Mode&07777 != 0550 || !workerACL(fd, uint32(os.Geteuid()), 5) {
				return CredentialRecord{}, ErrUnsafe
			}
		case 3:
			if s.Mode&unix.S_IFMT != unix.S_IFREG || s.Mode&07777 != 0440 || s.Nlink != 1 || s.Size < 1 || s.Size > 512 || !workerACL(fd, uint32(os.Geteuid()), 4) {
				return CredentialRecord{}, ErrUnsafe
			}
		}
	}
	f := os.NewFile(uintptr(fd), "service-credential")
	fd = -1
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 513))
	defer clear(b)
	if err != nil || len(b) > 512 {
		return CredentialRecord{}, ErrUnsafe
	}
	return DecodeCredential(b, server, connector)
}

func workerACL(fd int, uid uint32, permission uint16) bool {
	var data [45]byte
	n, err := unix.Fgetxattr(fd, "system.posix_acl_access", data[:])
	if err != nil || n != 44 {
		return false
	}
	if !exactACL(data[:44], uid, permission) {
		return false
	}
	n, err = unix.Fgetxattr(fd, "system.posix_acl_default", nil)
	return err == unix.ENODATA || err == unix.ENOTSUP || (err == nil && n == 0)
}

func exactACL(data []byte, uid uint32, permission uint16) bool {
	if len(data) != 44 || binary.LittleEndian.Uint32(data) != 2 {
		return false
	}
	tags := []uint16{1, 2, 4, 16, 32}
	perms := []uint16{permission, permission, 0, permission, 0}
	for i, tag := range tags {
		entry := data[4+i*8 : 12+i*8]
		id := ^uint32(0)
		if i == 1 {
			id = uid
		}
		if binary.LittleEndian.Uint16(entry) != tag || binary.LittleEndian.Uint16(entry[2:]) != perms[i] || binary.LittleEndian.Uint32(entry[4:]) != id {
			return false
		}
	}
	return true
}
