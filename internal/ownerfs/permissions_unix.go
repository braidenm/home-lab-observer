//go:build !windows

package ownerfs

import (
	"os"
	"syscall"
)

func RestrictDirectory(path string) error { return os.Chmod(path, 0o700) }
func RestrictFile(path string) error      { return os.Chmod(path, 0o600) }

func validateNotReparse(string) error { return nil }

func validateSingleLink(_ string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return ErrUnsafePath
	}
	return nil
}
