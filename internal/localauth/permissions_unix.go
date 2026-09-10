//go:build !windows

package localauth

import (
	"os"
	"syscall"
)

func restrictDirectory(path string) error { return os.Chmod(path, 0o700) }
func restrictFile(path string) error      { return os.Chmod(path, 0o600) }
func validPrivateTokenMode(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().Perm() == 0o600 && stat.Uid == uint32(os.Geteuid())
}
