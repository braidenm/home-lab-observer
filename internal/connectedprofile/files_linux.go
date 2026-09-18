//go:build linux

package connectedprofile

import (
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// ReadRootFile traverses only root-owned, non-writable real directories. No
// worker-controlled pathname or symbolic link becomes an installed authority.
func ReadRootFile(path string, limit int64) ([]byte, error) {
	return readRootFile(path, limit, 0)
}

// ReadRootFileMode additionally requires an exact installed file mode. It is
// for fixed, code-owned resources whose expected mode is part of admission.
func ReadRootFileMode(path string, limit int64, mode os.FileMode) ([]byte, error) {
	if mode != 0600 && mode != 0644 {
		return nil, ErrUnsafe
	}
	return readRootFile(path, limit, mode)
}

func readRootFile(path string, limit int64, mode os.FileMode) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || limit <= 0 || limit > 1024*1024 {
		return nil, ErrUnsafe
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer func() { unix.Close(fd) }()
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, ErrUnsafe
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		if e != nil {
			return nil, ErrUnsafe
		}
		unix.Close(fd)
		fd = next
		var s unix.Stat_t
		if unix.Fstat(fd, &s) != nil || s.Uid != 0 || s.Mode&0022 != 0 || s.Mode&07000 != 0 || !noACL(fd) {
			return nil, ErrUnsafe
		}
		if i == len(parts)-1 && (s.Mode&unix.S_IFMT != unix.S_IFREG || s.Nlink != 1 || s.Size > limit || s.Size < 1 ||
			(mode != 0 && os.FileMode(s.Mode&0777) != mode)) {
			return nil, ErrUnsafe
		}
	}
	f := os.NewFile(uintptr(fd), "installed-record")
	fd = -1
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, ErrUnsafe
	}
	return b, nil
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

func Load() (Config, error) {
	b, err := ReadRootFile(ConfigPath, MaxConfigBytes)
	if err != nil {
		return Config{}, ErrUnsafe
	}
	return Decode(b)
}
