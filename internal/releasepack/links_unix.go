//go:build linux || darwin

package releasepack

import (
	"io/fs"
	"syscall"
)

func singleLink(_ string, info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
