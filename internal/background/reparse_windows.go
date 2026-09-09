//go:build windows

package background

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func validatePlatformPath(path string) error {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `//`) {
		return errors.New("network and device paths are unsupported")
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' {
		return errors.New("path must use a local drive")
	}
	root, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return err
	}
	switch windows.GetDriveType(root) {
	case windows.DRIVE_FIXED, windows.DRIVE_REMOVABLE, windows.DRIVE_RAMDISK:
		return nil
	default:
		return errors.New("path must use a local disk")
	}
}

func rejectReparse(path string) error {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attributes, err := windows.GetFileAttributes(pointer)
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse point")
	}
	return nil
}
