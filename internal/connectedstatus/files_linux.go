//go:build linux

package connectedstatus

import (
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
)

type Writer struct {
	root             *os.Root
	directory, lease *os.File
}

func Open(path string) (*Writer, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafe
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnsafe
	}
	s, err := f.Stat()
	if err != nil || !s.IsDir() || s.Mode().Perm() != 0700 || s.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		f.Close()
		root.Close()
		return nil, ErrUnsafe
	}
	u, ok := s.Sys().(*syscall.Stat_t)
	if !ok || u.Uid != uint32(os.Geteuid()) || !noACL(int(f.Fd())) {
		f.Close()
		root.Close()
		return nil, ErrUnsafe
	}
	lease, err := openAt(f, ".status-lock", unix.O_RDWR|unix.O_CREAT, 0600)
	if err != nil {
		f.Close()
		root.Close()
		return nil, ErrUnsafe
	}
	w := &Writer{root, f, lease}
	if !privateFile(lease, 0) || unix.Flock(int(lease.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		w.Close()
		return nil, ErrUnsafe
	}
	entries, err := f.ReadDir(6)
	if (err != nil && !errors.Is(err, io.EOF)) || len(entries) > 5 {
		w.Close()
		return nil, ErrUnsafe
	}
	for _, e := range entries {
		if e.Name() != ".status-lock" && e.Name() != ".status-next" && e.Name() != "status.json" && e.Name() != "activation-response.json" && e.Name() != ".activation-next" {
			w.Close()
			return nil, ErrUnsafe
		}
		info, err := root.Lstat(e.Name())
		if err != nil || !info.Mode().IsRegular() {
			w.Close()
			return nil, ErrUnsafe
		}
		x, err := openAt(f, e.Name(), unix.O_RDONLY, 0)
		if err != nil {
			w.Close()
			return nil, ErrUnsafe
		}
		limit := int64(1024)
		if e.Name() == "activation-response.json" || e.Name() == ".activation-next" {
			limit = 2048
		}
		valid := privateFile(x, limit)
		x.Close()
		if !valid {
			w.Close()
			return nil, ErrUnsafe
		}
	}
	// Status is disposable, never delivery authority. Remove only the validated
	// fixed staging file under the exclusive status lease after a prior crash.
	for _, name := range []string{".status-next", ".activation-next"} {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			w.Close()
			return nil, ErrUnsafe
		}
	}
	return w, nil
}

func openAt(directory *os.File, name string, flags int, mode uint32) (*os.File, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, mode)
	if err != nil {
		return nil, ErrUnsafe
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (w *Writer) Write(r Record) error {
	b, err := Encode(r)
	if err != nil {
		return ErrUnsafe
	}
	return w.writeFixed(".status-next", "status.json", b)
}

// WriteActivationResponse publishes only a canonical closed response in the
// fixed private slot. A valid response remains diagnostic, not activation
// authority; the manager and worker must match their own live invocation.
func (w *Writer) WriteActivationResponse(data []byte) error {
	if len(data) == 0 || len(data) > connectedactivation.MaxBytes {
		return ErrUnsafe
	}
	closed := append([]byte(nil), data...)
	if _, err := connectedactivation.DecodeResponse(closed); err != nil {
		return ErrUnsafe
	}
	return w.writeFixed(".activation-next", "activation-response.json", closed)
}

func (w *Writer) writeFixed(stage, name string, b []byte) error {
	f, err := w.root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrUnsafe
	}
	if !privateFile(f, 0) {
		f.Close()
		return ErrUnsafe
	}
	n, err := f.Write(b)
	if err != nil || n != len(b) {
		f.Close()
		return ErrUnsafe
	}
	if f.Sync() != nil {
		f.Close()
		return ErrUnsafe
	}
	if f.Close() != nil || w.root.Rename(stage, name) != nil || w.directory.Sync() != nil {
		return ErrUnsafe
	}
	return nil
}

func (w *Writer) Close() error {
	a := w.directory.Close()
	b := w.root.Close()
	c := w.lease.Close()
	if a != nil || b != nil || c != nil {
		return ErrUnsafe
	}
	return nil
}

func privateFile(f *os.File, max int64) bool {
	s, err := f.Stat()
	if err != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || s.Size() > max {
		return false
	}
	u, ok := s.Sys().(*syscall.Stat_t)
	return ok && u.Uid == uint32(os.Geteuid()) && u.Nlink == 1 && noACL(int(f.Fd()))
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
