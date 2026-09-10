package logprocess

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const trustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
const maxHelperACEs = 256

func resolvePlatformHelper(executable, _ string) (commandSpec, error) {
	if !validWindowsExecutablePath(executable) {
		return commandSpec{}, ErrHelperUnavailable
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	current := user.User.Sid.String()
	before, err := verifyWindowsNode(executable, false, current)
	if err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	path := filepath.Dir(executable)
	finished := false
	for depth := 0; depth < maxIdentityDepth; depth++ {
		if _, err := verifyWindowsNode(path, true, current); err != nil {
			return commandSpec{}, ErrHelperUnavailable
		}
		parent := filepath.Dir(path)
		if parent == path {
			finished = true
			break
		}
		path = parent
	}
	if !finished {
		return commandSpec{}, ErrHelperUnavailable
	}
	after, err := verifyWindowsNode(executable, false, current)
	if err != nil || before != after {
		return commandSpec{}, ErrHelperMismatch
	}
	env, systemRoot, err := windowsHelperEnvironment()
	if err != nil || !validWindowsExecutablePath(systemRoot) {
		return commandSpec{}, ErrHelperUnavailable
	}
	// SystemRoot is an OS result, not an inherited environment value. Verify its
	// object as well; system-DLL loading remains required in the native binding.
	if _, err := verifyWindowsNode(systemRoot, true, current); err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	return commandSpec{path: executable, args: []string{"__log-helper"}, env: env}, nil
}

func windowsHelperEnvironment() ([]string, string, error) {
	root, err := windows.GetSystemWindowsDirectory()
	if err != nil || !validWindowsExecutablePath(root) {
		return nil, "", ErrHelperUnavailable
	}
	return append(helperEnvironment(), "SystemRoot="+root), root, nil
}

func validWindowsExecutablePath(path string) bool {
	volume := filepath.VolumeName(path)
	return len(path) > 3 && len(path) <= maxIdentityPathBytes && len(volume) == 2 && volume[1] == ':' &&
		(volume[0] >= 'A' && volume[0] <= 'Z' || volume[0] >= 'a' && volume[0] <= 'z') && filepath.IsAbs(path) && filepath.Clean(path) == path &&
		!strings.ContainsAny(path[2:], ":\x00")
}

type windowsFileIdentity struct {
	volume, high, low, sizeHigh, sizeLow uint32
	modified                             windows.Filetime
}

func verifyWindowsNode(path string, directory bool, current string) (windowsFileIdentity, error) {
	var zero windowsFileIdentity
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return zero, ErrHelperUnavailable
	}
	handle, err := windows.CreateFile(pointer, windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return zero, ErrHelperUnavailable
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || !safeWindowsFileInformation(info, directory) {
		return zero, ErrHelperUnavailable
	}
	typeID, err := windows.GetFileType(handle)
	if err != nil || typeID != windows.FILE_TYPE_DISK {
		return zero, ErrHelperUnavailable
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || !safeWindowsDescriptor(descriptor, directory, current) {
		return zero, ErrHelperUnavailable
	}
	return windowsFileIdentity{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.FileSizeHigh, info.FileSizeLow, info.LastWriteTime}, nil
}

func safeWindowsFileInformation(info windows.ByHandleFileInformation, directory bool) bool {
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return false
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	return directory || info.NumberOfLinks == 1 && size > 0 && size <= maxHelperBytes
}

func trustedWindowsSID(sid *windows.SID, current string) bool {
	if sid == nil || !sid.IsValid() {
		return false
	}
	value := sid.String()
	return value == current || value == "S-1-5-18" || value == "S-1-5-32-544" || value == trustedInstallerSID
}

func safeWindowsDescriptor(descriptor *windows.SECURITY_DESCRIPTOR, directory bool, current string) bool {
	if descriptor == nil || !descriptor.IsValid() || descriptor.Length() > 65536 {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || !trustedWindowsSID(owner, current) {
		return false
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount > maxHelperACEs {
		return false
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, index, &ace) != nil || ace == nil {
			return false
		}
		base := uintptr(unsafe.Pointer(descriptor))
		end := base + uintptr(descriptor.Length())
		address := uintptr(unsafe.Pointer(ace))
		if address < base || address > end-8 || ace.Header.AceSize < 8 || uintptr(ace.Header.AceSize) > end-address {
			return false
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceSize < 16 {
			return false
		}
		// IsValidSecurityDescriptor plus the explicit ACE/SID length checks keep
		// SID inspection inside the returned, bounded native ACL allocation.
		sidBytes := unsafe.Slice((*byte)(unsafe.Pointer(&ace.SidStart)), int(ace.Header.AceSize)-8)
		if sidBytes[0] != 1 || sidBytes[1] > 15 || 8+4*int(sidBytes[1]) > len(sidBytes) {
			return false
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.IsValid() {
			return false
		}
		if trustedWindowsSID(sid, current) || sid.IsWellKnown(windows.WinCreatorOwnerRightsSid) {
			// OWNER RIGHTS applies only to the already-validated object owner.
			continue
		}
		if unsafeWindowsGrant(uint32(ace.Mask), directory) {
			return false
		}
	}
	return true
}

func unsafeWindowsGrant(mask uint32, directory bool) bool {
	// Allow only understood read/execute rights and directory creation. Reject
	// generic write/all, MAXIMUM_ALLOWED and unknown bits rather than guessing.
	safe := uint32(windows.GENERIC_READ | windows.GENERIC_EXECUTE | windows.READ_CONTROL | windows.SYNCHRONIZE | windows.FILE_READ_DATA | windows.FILE_READ_EA | windows.FILE_READ_ATTRIBUTES | windows.FILE_EXECUTE)
	if directory {
		safe |= windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA
	}
	return mask & ^safe != 0
}
