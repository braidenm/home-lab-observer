//go:build darwin

package handoff

import (
	"os"
	"runtime"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Apple Libc acl_file.c uses acl_get_fd_np(ACL_TYPE_EXTENDED). statx_np.c
// clears absent FILESEC_ACL; filesec.c reports that absence as ENOENT.
// acl_size validates the object and reports header + entry bytes. Comparing
// with acl_init(0) avoids acl_get_entry's ambiguous -1/EINVAL empty/error result.
// Sources (accessed 2026-09-11): https://github.com/apple-oss-distributions/Libc
// /blob/main/{posix1e/acl_file.c,sys/statx_np.c,gen/filesec.c,
// posix1e/acl_translate.c,include/sys/acl.h}.
type aclCalls struct {
	getFD func(int32) uintptr
	init  func(int32) uintptr
	size  func(uintptr) int64
	free  func(uintptr) int32
	errno func() *int32
}

func noExtendedACL(file *os.File) (result error) {
	if file == nil {
		return ErrUnsafe
	}
	calls, closeLibrary, err := loadACLCalls()
	if err != nil {
		return ErrUnsafe
	}
	defer func() {
		if closeLibrary() != nil {
			result = ErrUnsafe
		}
	}()
	raw, err := file.SyscallConn()
	if err != nil {
		return ErrUnsafe
	}
	result = ErrUnsafe
	// Control prevents Close from invalidating/reusing this descriptor mid-check.
	if raw.Control(func(fd uintptr) {
		if fd > 1<<31-1 {
			return
		}
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		result = checkExtendedACL(int32(fd), calls)
	}) != nil {
		return ErrUnsafe
	}
	return result
}

// Caller pins the OS thread: errno belongs to that thread, not the goroutine.
func checkExtendedACL(fd int32, calls aclCalls) (result error) {
	result = ErrUnsafe
	errno := calls.errno()
	if errno == nil {
		return result
	}
	*errno = 0
	acl := calls.getFD(fd)
	getErrno := *errno
	if acl == 0 {
		if getErrno == int32(unix.ENOENT) {
			return nil
		}
		return result
	}
	// Refuse special sentinel values, including _FILESEC_REMOVE_ACL. Only actual
	// objects returned by the library may reach acl_size/acl_free.
	if !aclObject(acl) {
		return result
	}
	defer func() {
		if calls.free(acl) != 0 {
			result = ErrUnsafe
		}
	}()
	empty := calls.init(0)
	if !aclObject(empty) {
		return result
	}
	defer func() {
		if calls.free(empty) != 0 {
			result = ErrUnsafe
		}
	}()
	emptySize, actualSize := calls.size(empty), calls.size(acl)
	if emptySize > 0 && actualSize == emptySize {
		return nil
	}
	return result
}

func aclObject(value uintptr) bool { return value > 16 && value < ^uintptr(0)-16 }

func loadACLCalls() (calls aclCalls, closeLibrary func() error, err error) {
	library, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return aclCalls{}, nil, ErrUnsafe
	}
	closeLibrary = func() error { return purego.Dlclose(library) }
	bindings := []struct {
		name   string
		target any
	}{{"acl_get_fd", &calls.getFD}, {"acl_init", &calls.init}, {"acl_size", &calls.size},
		{"acl_free", &calls.free}, {"__error", &calls.errno}}
	// Recover only registration failures, never native-call faults or caller code.
	defer func() {
		if recover() != nil {
			_ = closeLibrary()
			calls, closeLibrary, err = aclCalls{}, nil, ErrUnsafe
		}
	}()
	for _, binding := range bindings {
		address, lookupErr := purego.Dlsym(library, binding.name)
		if lookupErr != nil || address == 0 {
			_ = closeLibrary()
			return aclCalls{}, nil, ErrUnsafe
		}
		purego.RegisterFunc(binding.target, address)
	}
	return calls, closeLibrary, nil
}
