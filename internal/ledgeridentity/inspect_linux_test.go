//go:build linux && (amd64 || arm64)

package ledgeridentity

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func fakeOps(hook func(string, int, int)) nativeOps {
	counts := map[string]int{}
	call := func(name string, fd int) {
		counts[name]++
		if hook != nil {
			hook(name, fd, counts[name])
		}
	}
	return nativeOps{
		statfs: func(fd int, fs *unix.Statfs_t) error { call("fs", fd); fs.Type = unix.EXT4_SUPER_MAGIC; return nil },
		stat: func(fd int, st *unix.Stat_t) error {
			call("stat", fd)
			st.Dev = 1
			st.Ino = uint64(fd)
			st.Nlink = 1
			st.Mode = unix.S_IFREG | 0600
			if fd == 1 {
				st.Mode = unix.S_IFDIR | 0700
			}
			return nil
		},
		uuid:       func(fd int) ([16]byte, error) { call("uuid", fd); return [16]byte{1}, nil },
		generation: func(fd int) (uint32, error) { call("generation", fd); return 7, nil },
	}
}

func TestFixedQueriesAndFaults(t *testing.T) {
	var calls []string
	ops := fakeOps(func(name string, _ int, _ int) { calls = append(calls, name) })
	w, err := inspect(context.Background(), 1, 2, ops)
	if err != nil || Validate(w) != nil || len(calls) != 16 {
		t.Fatal("bounded inspection failed")
	}
	for _, operation := range []string{"fs", "stat", "uuid", "generation"} {
		for failAt := 1; failAt <= 4; failAt++ {
			ops := fakeOps(nil)
			n := 0
			failure := errors.New("private native diagnostic")
			switch operation {
			case "fs":
				old := ops.statfs
				ops.statfs = func(fd int, s *unix.Statfs_t) error {
					n++
					if n == failAt {
						return failure
					}
					return old(fd, s)
				}
			case "stat":
				old := ops.stat
				ops.stat = func(fd int, s *unix.Stat_t) error {
					n++
					if n == failAt {
						return failure
					}
					return old(fd, s)
				}
			case "uuid":
				old := ops.uuid
				ops.uuid = func(fd int) ([16]byte, error) {
					n++
					if n == failAt {
						return [16]byte{}, failure
					}
					return old(fd)
				}
			case "generation":
				old := ops.generation
				ops.generation = func(fd int) (uint32, error) {
					n++
					if n == failAt {
						return 0, failure
					}
					return old(fd)
				}
			}
			if w, err := inspect(context.Background(), 1, 2, ops); err != ErrUnavailable || w != (Witness{}) {
				t.Fatal("native error leaked or accepted")
			}
		}
	}
}

func TestUUIDIoctlBufferContract(t *testing.T) {
	if unsafe.Sizeof(uuidBuffer{}) != 24 || unsafe.Offsetof(uuidBuffer{}.UUID) != 8 ||
		getFilesystemUUID != 0x8008662c || getGeneration != 0x80086603 {
		t.Fatal("unexpected ioctl layout")
	}
	for _, mode := range []string{"valid", "length", "flags", "failure"} {
		got, err := readUUIDWith(17, func(fd int, b *uuidBuffer) error {
			if fd != 17 || b.Length != 16 || b.Flags != 0 || b.UUID != [16]byte{} {
				t.Fatal("unbounded or uninitialized UUID request")
			}
			b.UUID[0] = 1
			switch mode {
			case "length":
				b.Length = 17
			case "flags":
				b.Flags = 1
			case "failure":
				return errors.New("private error")
			}
			return nil
		})
		if mode == "valid" {
			if err != nil || got != [16]byte{1} {
				t.Fatal("valid UUID refused")
			}
		} else if err != ErrUnavailable || got != [16]byte{} {
			t.Fatal("invalid UUID response accepted")
		}
	}
}

func TestRejectIdentityAndMetadataDrift(t *testing.T) {
	for _, name := range []string{"filesystem", "device", "different-uuid", "uuid", "zero-uuid", "generation", "zero-generation", "inode", "mode", "uid", "gid", "links", "unlinked", "hardlink", "wrong-type"} {
		t.Run(name, func(t *testing.T) {
			ops := fakeOps(nil)
			statCalls, uuidCalls, genCalls := 0, 0, 0
			stat := ops.stat
			ops.stat = func(fd int, s *unix.Stat_t) error {
				_ = stat(fd, s)
				statCalls++
				if name == "device" && fd == 2 {
					s.Dev++
				}
				if name == "unlinked" {
					s.Nlink = 0
				}
				if name == "hardlink" && fd == 2 {
					s.Nlink = 2
				}
				if name == "wrong-type" && fd == 2 {
					s.Mode = unix.S_IFIFO | 0600
				}
				if statCalls == 3 {
					switch name {
					case "inode":
						s.Ino++
					case "mode":
						s.Mode ^= 0040
					case "uid":
						s.Uid++
					case "gid":
						s.Gid++
					case "links":
						s.Nlink++
					}
				}
				return nil
			}
			if name == "filesystem" {
				ops.statfs = func(_ int, s *unix.Statfs_t) error { s.Type = unix.TMPFS_MAGIC; return nil }
			}
			ops.uuid = func(fd int) ([16]byte, error) {
				uuidCalls++
				u := [16]byte{1}
				if name == "zero-uuid" {
					u = [16]byte{}
				}
				if name == "uuid" && uuidCalls == 3 {
					u[0]++
				}
				if name == "different-uuid" && fd == 2 {
					u[0]++
				}
				return u, nil
			}
			ops.generation = func(int) (uint32, error) {
				genCalls++
				if name == "zero-generation" {
					return 0, nil
				}
				if name == "generation" && genCalls == 4 {
					return 8, nil
				}
				return 7, nil
			}
			if w, err := inspect(context.Background(), 1, 2, ops); err != ErrUnavailable || w != (Witness{}) {
				t.Fatal("unsupported or changed identity accepted")
			}
		})
	}
}

func TestCancellationBetweenEveryNativeOperation(t *testing.T) {
	for cancelAt := 0; cancelAt <= 16; cancelAt++ {
		ctx, cancel := context.WithCancel(context.Background())
		n := 0
		if cancelAt == 0 {
			cancel()
		}
		ops := fakeOps(func(string, int, int) {
			n++
			if n == cancelAt {
				cancel()
			}
		})
		w, err := inspect(ctx, 1, 2, ops)
		cancel()
		if err != ErrUnavailable || w != (Witness{}) {
			t.Fatal("canceled inspection accepted")
		}
	}
}

func TestNativeOwnedExt4(t *testing.T) {
	dir := t.TempDir()
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal("fixture directory failed")
	}
	defer d.Close()
	b, err := os.OpenFile(filepath.Join(dir, "ledger"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("fixture file failed")
	}
	defer b.Close()
	w, err := Inspect(context.Background(), d, b)
	if err != nil {
		if os.Getenv("OBSERVER_EXT4_ACCEPTANCE") == "1" {
			t.Fatal("required native ext4 identity unsupported")
		}
		t.Skip("native ext4 ioctl profile unavailable")
	}
	if unsafe.Sizeof(uuidBuffer{}) != 24 || unsafe.Offsetof(uuidBuffer{}.UUID) != 8 {
		t.Fatal("unexpected ioctl layout")
	}
	for range 3 {
		again, err := Inspect(context.Background(), d, b)
		if err != nil || again != w {
			t.Fatal("repeat identity changed")
		}
	}
	if _, err := d.Stat(); err != nil {
		t.Fatal("caller directory closed")
	}
	if _, err := b.Stat(); err != nil {
		t.Fatal("caller file closed")
	}
	if err := os.Rename(filepath.Join(dir, "ledger"), filepath.Join(dir, "old")); err != nil {
		t.Fatal("fixture rename failed")
	}
	replacement, err := os.OpenFile(filepath.Join(dir, "ledger"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("fixture replacement failed")
	}
	defer replacement.Close()
	other, err := Inspect(context.Background(), d, replacement)
	if err != nil || other == w {
		t.Fatal("replacement identity not distinguished")
	}
	again, err := Inspect(context.Background(), d, b)
	if err != nil || again != w {
		t.Fatal("anchor followed replaced name")
	}
	if err := os.Remove(filepath.Join(dir, "old")); err != nil {
		t.Fatal("fixture unlink failed")
	}
	if got, err := Inspect(context.Background(), d, b); err != ErrUnavailable || got != (Witness{}) {
		t.Fatal("unlinked database accepted")
	}
}

func TestInvalidOwnedHandles(t *testing.T) {
	if w, e := Inspect(nil, nil, nil); e != ErrUnavailable || w != (Witness{}) {
		t.Fatal("nil input accepted")
	}
	d, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal("fixture open failed")
	}
	defer d.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal("fixture pipe failed")
	}
	defer r.Close()
	defer w.Close()
	if got, e := Inspect(context.Background(), d, r); e != ErrUnavailable || got != (Witness{}) {
		t.Fatal("pipe accepted")
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("fixture socket failed")
	}
	s := os.NewFile(uintptr(fds[0]), "synthetic")
	defer s.Close()
	defer unix.Close(fds[1])
	if got, e := Inspect(context.Background(), d, s); e != ErrUnavailable || got != (Witness{}) {
		t.Fatal("socket accepted")
	}
	if err := r.Close(); err != nil {
		t.Fatal("fixture close failed")
	}
	if got, e := Inspect(context.Background(), d, r); e != ErrUnavailable || got != (Witness{}) {
		t.Fatal("closed handle accepted")
	}
}
