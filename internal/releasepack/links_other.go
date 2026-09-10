//go:build !linux && !darwin && !windows

package releasepack

import "io/fs"

func singleLink(string, fs.FileInfo) bool { return false }
