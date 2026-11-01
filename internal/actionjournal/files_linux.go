//go:build linux

package actionjournal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
	"golang.org/x/sys/unix"
)

func privateDirectory(path string) (string, error) {
	path, e := filepath.Abs(path)
	if e != nil || ownerfs.ValidateDedicatedDirectory(path) != nil {
		return "", ErrUnsafe
	}
	for p := path; ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafe
		}
		s, ok := i.Sys().(*syscall.Stat_t)
		if !ok || (s.Uid != 0 && s.Uid != uint32(os.Geteuid())) {
			return "", ErrUnsafe
		}
		if p == path {
			if s.Uid != uint32(os.Geteuid()) || i.Mode().Perm() != 0700 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
				return "", ErrUnsafe
			}
		} else if i.Mode().Perm()&0022 != 0 && i.Mode()&os.ModeSticky == 0 {
			return "", ErrUnsafe
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return path, nil
}
func privateFile(i os.FileInfo, limit int64) bool {
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid()) && s.Nlink == 1 && i.Mode().IsRegular() && i.Mode().Perm() == 0600 && i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 && i.Size() >= 0 && i.Size() <= limit
}
func checkFiles(dir string, dbRequired bool) error {
	f, e := os.Open(dir)
	if e != nil {
		return ErrUnsafe
	}
	defer f.Close()
	entries, e := f.ReadDir(4)
	if (e != nil && !errors.Is(e, io.EOF)) || len(entries) > 3 {
		return ErrUnsafe
	}
	var total int64
	db, lock := false, false
	for _, v := range entries {
		limit := int64(totalBytes)
		switch v.Name() {
		case databaseName:
			limit = databaseBytes
			db = true
		case lockName:
			limit = 0
			lock = true
		case journalName:
			limit = journalBytes
		default:
			return ErrUnsafe
		}
		i, e := os.Lstat(filepath.Join(dir, v.Name()))
		if e != nil || !privateFile(i, limit) {
			return ErrUnsafe
		}
		total += i.Size()
	}
	if !lock || (dbRequired && !db) || total > totalBytes {
		return ErrRecovery
	}
	return nil
}
func acquireLock(dir string, create bool) (*os.File, error) {
	flags := os.O_RDWR | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	}
	f, e := os.OpenFile(filepath.Join(dir, lockName), flags, 0600)
	if e != nil {
		return nil, ErrRecovery
	}
	i, e := f.Stat()
	if e != nil || !privateFile(i, 0) {
		f.Close()
		return nil, ErrUnsafe
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		if errors.Is(e, unix.EWOULDBLOCK) || errors.Is(e, unix.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, ErrRecovery
	}
	return f, nil
}
func createDatabase(dir string) error {
	f, e := os.OpenFile(filepath.Join(dir, databaseName), os.O_CREATE|os.O_EXCL|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return ErrRecovery
	}
	if f.Close() != nil {
		return ErrRecovery
	}
	return nil
}
func checkHeader(dir string) error {
	f, e := os.OpenFile(filepath.Join(dir, databaseName), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if e != nil {
		return ErrRecovery
	}
	defer f.Close()
	i, e := f.Stat()
	if e != nil || !privateFile(i, databaseBytes) {
		return ErrUnsafe
	}
	var h [100]byte
	if _, e = io.ReadFull(f, h[:]); e != nil || !bytes.Equal(h[:16], []byte("SQLite format 3\x00")) || h[16] != 16 || h[17] != 0 || h[18] != 1 || h[19] != 1 || binary.BigEndian.Uint32(h[60:64]) != 1 || binary.BigEndian.Uint32(h[68:72]) != 0x484C414A {
		return ErrRecovery
	}
	return nil
}

// Only a bounded first-header envelope is inspected before SQL owns the files.
// SQLite, not this adapter, validates checksums, additional segments and recovery.
func checkJournalHeader(dir string) error {
	f, e := os.OpenFile(filepath.Join(dir, journalName), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return ErrRecovery
	}
	defer f.Close()
	i, e := f.Stat()
	if e != nil || !privateFile(i, journalBytes) {
		return ErrUnsafe
	}
	if i.Size() == 0 {
		return nil
	}
	var h [28]byte
	n, e := io.ReadFull(f, h[:])
	if e != nil {
		if (e == io.EOF || e == io.ErrUnexpectedEOF) && bytes.Equal(h[:n], make([]byte, n)) {
			return nil
		}
		return ErrRecovery
	}
	cold := bytes.Equal(h[:8], make([]byte, 8))
	hot := bytes.Equal(h[:8], []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7})
	if !cold && !hot {
		return ErrRecovery
	}
	if cold && bytes.Equal(h[:], make([]byte, 28)) {
		return nil
	}
	count := binary.BigEndian.Uint32(h[8:12])
	pages := binary.BigEndian.Uint32(h[16:20])
	sector := binary.BigEndian.Uint32(h[20:24])
	size := binary.BigEndian.Uint32(h[24:28])
	if pages < 1 || pages > 3072 || sector < 512 || sector > 65536 || sector&(sector-1) != 0 || size != 4096 || i.Size() < int64(sector) {
		return ErrRecovery
	}
	if count == ^uint32(0) {
		if (i.Size()-int64(sector))/(4096+8) > 3072 {
			return ErrRecovery
		}
	} else if count > pages || count > 3072 || (hot && i.Size() < int64(sector)+int64(count)*(4096+8)) {
		return ErrRecovery
	}
	return nil
}
func syncDirectory(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return ErrRecovery
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
