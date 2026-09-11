//go:build windows

package handoff

import (
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

func privateHandle(file *os.File, directory bool) error {
	handle := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnsafe
	}
	if (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || (!directory && info.NumberOfLinks != 1) {
		return ErrUnsafe
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ErrUnsafe
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil || !sd.IsValid() {
		return ErrUnsafe
	}
	defer runtime.KeepAlive(sd)
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return ErrUnsafe
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return ErrUnsafe
	}
	// x/sys GetSecurityInfo returns a Go-owned self-relative descriptor. Bound
	// its ACL span before inspecting any variable-length ACE or SID bytes.
	control, _, err := sd.Control()
	length := uintptr(sd.Length())
	base, address := uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(acl))
	if err != nil || control&windows.SE_SELF_RELATIVE == 0 || length < 20 || length > 1<<20 || address < base || address-base > length-8 {
		return ErrUnsafe
	}
	remaining := length - (address - base)
	header := unsafe.Slice((*byte)(unsafe.Pointer(acl)), 8)
	aclSize := uintptr(binary.LittleEndian.Uint16(header[2:4]))
	if aclSize < 8 || aclSize > remaining {
		return ErrUnsafe
	}
	return privateACLBytes(unsafe.Slice((*byte)(unsafe.Pointer(acl)), int(aclSize)), user.User.Sid)
}

// Layouts are the documented Windows ACL, ACE_HEADER, ACCESS_ALLOWED_ACE and
// SID structures. Validate each boundary before passing a SID to native APIs.
func privateACLBytes(data []byte, owner *windows.SID) error {
	if len(data) < 8 || (data[0] != 2 && data[0] != 4) || int(binary.LittleEndian.Uint16(data[2:4])) != len(data) || owner == nil {
		return ErrUnsafe
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 16 {
		return ErrUnsafe
	}
	offset := 8
	for i := 0; i < count; i++ {
		if len(data)-offset < 4 {
			return ErrUnsafe
		}
		size := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
		if size < 16 || size%4 != 0 || size > len(data)-offset || data[offset] != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnsafe
		}
		ace := data[offset : offset+size]
		// SID revision + count + six-byte authority, then count uint32 values.
		if ace[8] != 1 || ace[9] > 15 || 8+8+4*int(ace[9]) != len(ace) {
			return ErrUnsafe
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace[8]))
		if !sid.IsValid() || (!sid.Equals(owner) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)) {
			return ErrUnsafe
		}
		offset += size
	}
	return nil
}

func safeOpenFlags() int { return 0 }

var reopenCreatedFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

// The caller proves exclusive creation. Go's original handle lacks WRITE_OWNER;
// ReOpenFile requests it for the same file object without resolving a pathname.
func prepareCreatedFile(file *os.File) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || reopenCreatedFile.Find() != nil {
		return ErrUnsafe
	}
	raw, err := file.SyscallConn()
	if err != nil {
		return ErrUnsafe
	}
	result := ErrUnsafe
	if raw.Control(func(fd uintptr) {
		h, _, _ := reopenCreatedFile.Call(fd, windows.WRITE_OWNER|windows.READ_CONTROL,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, 0)
		if h == uintptr(windows.InvalidHandle) || h == 0 {
			return
		}
		err := windows.SetSecurityInfo(windows.Handle(h), windows.SE_FILE_OBJECT,
			windows.OWNER_SECURITY_INFORMATION, user.User.Sid, nil, nil, nil)
		closeErr := windows.CloseHandle(windows.Handle(h))
		if err == nil && closeErr == nil {
			result = nil
		}
	}) != nil {
		return ErrUnsafe
	}
	return result
}

// Windows does not offer the same directory-fsync guarantee as Unix here.
// The slot is disposable; no durable upload or action identity is stored in it.
func syncDirectory(*os.File) error { return nil }
func lockWriter(file *os.File) error {
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrBusy
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
