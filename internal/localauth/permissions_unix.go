//go:build !windows

package localauth

import "os"

func restrictDirectory(path string) error                   { return os.Chmod(path, 0o700) }
func restrictFile(path string) error                        { return os.Chmod(path, 0o600) }
func validPrivateTokenMode(_ string, info os.FileInfo) bool { return info.Mode().Perm() == 0o600 }
