//go:build linux

package connectedinstall

import (
	"os"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

func prepareActionCredentialLock(i actionprofile.Identity) error {
	if i.WorkerUID == 0 || i.WorkerGID == 0 {
		return ErrUnsafe
	}
	parent, err := rootDirectory(actionStateRoot)
	if err != nil {
		return ErrRecovery
	}
	defer parent.Close()
	dir, err := unix.Openat(int(parent.Fd()), "worker", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrRecovery
	}
	worker := os.NewFile(uintptr(dir), "action-worker-directory")
	defer worker.Close()
	if !exactDirectory(worker, directorySpec{uid: i.WorkerUID, gid: i.WorkerGID, mode: 0700}) {
		return ErrUnsafe
	}
	fd, err := unix.Openat(dir, "credential.lock", unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_CREAT|unix.O_EXCL, 0600)
	if err != nil {
		return ErrRecovery
	}
	lock := os.NewFile(uintptr(fd), "action-credential-lock")
	defer lock.Close()
	if lock.Chown(int(i.WorkerUID), int(i.WorkerGID)) != nil || lock.Chmod(0600) != nil ||
		!regularOwned(lock, i.WorkerUID, i.WorkerGID, 0600, 0) || lock.Sync() != nil || worker.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
