//go:build linux

package connectedpreparing

import (
	"os"

	"golang.org/x/sys/unix"
)

const configDirectoryName = "home-lab-observer-connected"

// CreateConfigDirectory creates only the fixed first-install config root.
// A future installer must hold the lease and prove no prior installation.
func CreateConfigDirectory() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	etc, err := openTrustedEtc()
	if err != nil {
		return err
	}
	defer etc.Close()
	return createAt(etc, nil)
}

// createAt is only a synthetic-root fixture seam. Existing entries and all
// partial creations remain in place for explicit recovery, never adoption.
func createAt(etc *os.File, syncFile func(*os.File) error) (result error) {
	if etc == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !trustedDirectory(etc, false) {
		return ErrUnsafe
	}
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	if unix.Mkdirat(int(etc.Fd()), configDirectoryName, 0700) != nil {
		return ErrRecovery
	}
	config, err := openDirAt(int(etc.Fd()), configDirectoryName)
	if err != nil {
		return ErrRecovery
	}
	defer func() {
		if config.Close() != nil && result == nil {
			result = ErrRecovery
		}
	}()
	if config.Chown(0, 0) != nil || config.Chmod(0755) != nil || !trustedDirectory(config, true) ||
		syncFile(config) != nil || syncFile(etc) != nil {
		return ErrRecovery
	}
	return nil
}
