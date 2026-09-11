//go:build linux

package uploadledger

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

// Directory ancestry is checked without changing existing permissions. A sticky
// root/current-owned ancestor permits temporary-directory fixtures safely.
func privateDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil || ownerfs.ValidateDedicatedDirectory(abs) != nil {
		return "", ErrUnsafe
	}
	for current := abs; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafe
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
			return "", ErrUnsafe
		}
		if current == abs {
			if st.Uid != uint32(os.Geteuid()) || info.Mode().Perm() != 0o700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				return "", ErrUnsafe
			}
		} else if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return "", ErrUnsafe
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return abs, nil
}

func privateFile(info os.FileInfo, limit int64) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid()) && st.Nlink == 1 && info.Mode().IsRegular() &&
		info.Mode().Perm() == 0o600 && info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 && info.Size() >= 0 && info.Size() <= limit
}

// Uses metadata only; never opens/closes the database while SQLite owns it.
func checkFiles(dir string, requireDatabase bool) error {
	f, err := os.Open(dir)
	if err != nil {
		return ErrUnsafe
	}
	defer f.Close()
	entries, err := f.ReadDir(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return ErrUnsafe
	}
	if len(entries) > 3 {
		return ErrUnsafe
	}
	var total int64
	var database, lock bool
	for _, entry := range entries {
		limit := int64(maxTotalBytes)
		switch entry.Name() {
		case databaseName:
			limit = maxDatabaseBytes
			database = true
		case lockName:
			limit = 0
			lock = true
		case journalName:
		default:
			return ErrUnsafe
		}
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !privateFile(info, limit) {
			return ErrUnsafe
		}
		total += info.Size()
	}
	if !lock || (requireDatabase && !database) || total > maxTotalBytes {
		return ErrRecovery
	}
	return nil
}

func acquireLock(dir string, create bool) (*os.File, error) {
	flags := os.O_RDWR | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), flags, 0o600)
	if err != nil {
		return nil, ErrRecovery
	}
	info, err := f.Stat()
	if err != nil || !privateFile(info, 0) {
		f.Close()
		return nil, ErrUnsafe
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, ErrRecovery
	}
	return f, nil
}

func createDatabase(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, databaseName), os.O_CREATE|os.O_EXCL|os.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return ErrRecovery
	}
	if err = f.Close(); err != nil {
		return ErrRecovery
	}
	return nil
}

// Called only after acquiring the lease and before opening any SQL connection.
// Refuse foreign page/journal formats before SQLite could migrate or create sidecars.
func checkHeader(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, databaseName), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return ErrRecovery
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !privateFile(info, maxDatabaseBytes) {
		return ErrUnsafe
	}
	var header [100]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return ErrRecovery
	}
	if !bytes.Equal(header[:16], []byte("SQLite format 3\x00")) || header[16] != 16 || header[17] != 0 || header[18] != 1 || header[19] != 1 {
		return ErrRecovery
	}
	return nil
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return ErrRecovery
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
