//go:build linux && (amd64 || arm64)

package ledgeridentity

import (
	"context"
	"encoding/hex"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux ext4 UAPI: _IOR('f',44,struct fsuuid) encodes the 8-byte header,
// not the following UUID; GETVERSION encodes long but returns a 32-bit value.
const getFilesystemUUID = 0x8008662c
const getGeneration = 0x80086603

type uuidBuffer struct {
	Length uint32
	Flags  uint32
	UUID   [16]byte
}

type nativeOps struct {
	statfs     func(int, *unix.Statfs_t) error
	stat       func(int, *unix.Stat_t) error
	uuid       func(int) ([16]byte, error)
	generation func(int) (uint32, error)
}

var systemOps = nativeOps{unix.Fstatfs, unix.Fstat, readUUID,
	func(fd int) (uint32, error) { return unix.IoctlGetUint32(fd, getGeneration) }}

func readUUID(fd int) ([16]byte, error) {
	return readUUIDWith(fd, func(fd int, b *uuidBuffer) error {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), getFilesystemUUID, uintptr(unsafe.Pointer(b)))
		if errno != 0 {
			return ErrUnavailable
		}
		return nil
	})
}

func readUUIDWith(fd int, ioctl func(int, *uuidBuffer) error) ([16]byte, error) {
	b := uuidBuffer{Length: 16}
	if ioctl(fd, &b) != nil || b.Length != 16 || b.Flags != 0 {
		return [16]byte{}, ErrUnavailable
	}
	return b.UUID, nil
}

// Inspect pins both supplied descriptors. The caller establishes trusted path,
// ownership/ACL policy, directory membership and stopped writers before calling.
// Context checks cannot interrupt an individual filesystem syscall.
func Inspect(ctx context.Context, directory, database *os.File) (Witness, error) {
	if ctx == nil || ctx.Err() != nil || directory == nil || database == nil {
		return Witness{}, ErrUnavailable
	}
	dir, err := directory.SyscallConn()
	if err != nil {
		return Witness{}, ErrUnavailable
	}
	db, err := database.SyscallConn()
	if err != nil {
		return Witness{}, ErrUnavailable
	}
	var result Witness
	var inner, inspected error
	err = dir.Control(func(dfd uintptr) {
		inner = db.Control(func(bfd uintptr) {
			result, inspected = inspect(ctx, int(dfd), int(bfd), systemOps)
		})
	})
	if err != nil || inner != nil || inspected != nil || ctx.Err() != nil {
		return Witness{}, ErrUnavailable
	}
	return result, nil
}

type facts struct {
	device     uint64
	inode      uint64
	mode       uint32
	uid, gid   uint32
	links      uint64
	uuid       [16]byte
	generation uint32
}

func observe(ctx context.Context, fd int, directory bool, ops nativeOps) (facts, error) {
	var fs unix.Statfs_t
	var st unix.Stat_t
	var f facts
	if ctx.Err() != nil || ops.statfs(fd, &fs) != nil || fs.Type != unix.EXT4_SUPER_MAGIC ||
		ctx.Err() != nil || ops.stat(fd, &st) != nil {
		return f, ErrUnavailable
	}
	kind := uint32(unix.S_IFREG)
	if directory {
		kind = unix.S_IFDIR
	}
	if st.Mode&unix.S_IFMT != kind || st.Ino == 0 || st.Nlink == 0 || (!directory && st.Nlink != 1) {
		return f, ErrUnavailable
	}
	f = facts{device: st.Dev, inode: st.Ino, mode: st.Mode, uid: st.Uid, gid: st.Gid, links: uint64(st.Nlink)}
	var err error
	if ctx.Err() != nil {
		return facts{}, ErrUnavailable
	}
	f.uuid, err = ops.uuid(fd)
	if err != nil || f.uuid == [16]byte{} || ctx.Err() != nil {
		return facts{}, ErrUnavailable
	}
	f.generation, err = ops.generation(fd)
	if err != nil || f.generation == 0 || ctx.Err() != nil {
		return facts{}, ErrUnavailable
	}
	return f, nil
}

func inspect(ctx context.Context, dir, db int, ops nativeOps) (Witness, error) {
	a, err := observe(ctx, dir, true, ops)
	if err != nil {
		return Witness{}, ErrUnavailable
	}
	b, err := observe(ctx, db, false, ops)
	if err != nil || a.device != b.device || a.uuid != b.uuid {
		return Witness{}, ErrUnavailable
	}
	afterA, err := observe(ctx, dir, true, ops)
	if err != nil || afterA != a {
		return Witness{}, ErrUnavailable
	}
	afterB, err := observe(ctx, db, false, ops)
	if err != nil || afterB != b {
		return Witness{}, ErrUnavailable
	}
	w := Witness{FilesystemUUID: hex.EncodeToString(a.uuid[:]), Directory: Object{a.inode, a.generation}, Database: Object{b.inode, b.generation}}
	if Validate(w) != nil {
		return Witness{}, ErrUnavailable
	}
	return w, nil
}
