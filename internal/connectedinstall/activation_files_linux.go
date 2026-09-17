//go:build linux

package connectedinstall

import (
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const activationParent = "/run/home-lab-observer-connected"
const activationPath = activationParent + "/activation"

// pinActivation operates only with both workers stopped and the installation
// lease held. Unknown files and interrupted staging require explicit recovery.
func pinActivation(gid uint32) (*os.File, error) {
	ancestor, err := rootDirectory("/run")
	if err != nil {
		return nil, ErrUnsafe
	}
	ancestor.Close()
	for _, item := range []struct {
		path  string
		group uint32
		mode  os.FileMode
	}{{activationParent, 0, 0700}, {activationPath, gid, 0750}} {
		if _, err := os.Lstat(item.path); os.IsNotExist(err) {
			if mkdirNew(item.path, 0, int(item.group), item.mode) != nil {
				return nil, ErrRecovery
			}
		} else if err != nil {
			return nil, ErrUnsafe
		}
		f, err := rootDirectory(item.path)
		if err != nil {
			return nil, ErrUnsafe
		}
		valid := activationDirectory(f, item.group, item.mode)
		f.Close()
		if !valid {
			return nil, ErrUnsafe
		}
	}
	d, err := rootDirectory(activationPath)
	if err != nil {
		return nil, ErrUnsafe
	}
	names, err := d.Readdirnames(3)
	if err != nil && err != io.EOF || len(names) > 2 {
		d.Close()
		return nil, ErrUnsafe
	}
	for _, name := range names {
		if name != "request.json" && name != "commit.json" {
			d.Close()
			return nil, ErrUnsafe
		}
		b, err := activationRecord(d, name, 0, gid, 0640)
		if err != nil {
			d.Close()
			return nil, ErrUnsafe
		}
		if name == "request.json" {
			_, err = connectedactivation.DecodeRequest(b)
		} else {
			_, err = connectedactivation.DecodeCommit(b)
		}
		if err != nil || unix.Unlinkat(int(d.Fd()), name, 0) != nil {
			d.Close()
			return nil, ErrRecovery
		}
	}
	if d.Sync() != nil {
		d.Close()
		return nil, ErrRecovery
	}
	return d, nil
}

func activationDirectory(f *os.File, gid uint32, mode os.FileMode) bool {
	var st unix.Stat_t
	return unix.Fstat(int(f.Fd()), &st) == nil && st.Uid == 0 && st.Gid == gid && st.Mode&unix.S_IFMT == unix.S_IFDIR && st.Mode&07777 == uint32(mode) && noACL(int(f.Fd()))
}

func sameActivationDirectory(d *os.File, gid uint32) bool {
	current, err := rootDirectory(activationPath)
	if err != nil {
		return false
	}
	defer current.Close()
	a, e1 := d.Stat()
	b, e2 := current.Stat()
	return e1 == nil && e2 == nil && os.SameFile(a, b) && activationDirectory(current, gid, 0750)
}

func publishActivation(d *os.File, name string, b []byte, gid uint32) error {
	if (name != "request.json" && name != "commit.json") || len(b) == 0 || len(b) > connectedactivation.MaxBytes || !sameActivationDirectory(d, gid) {
		return ErrUnsafe
	}
	fd, err := unix.Openat(int(d.Fd()), ".activation-next", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrRecovery
	}
	f := os.NewFile(uintptr(fd), "activation-record")
	if f.Chown(0, int(gid)) != nil || f.Chmod(0640) != nil || !regular(f, 0, 0640, 0) {
		f.Close()
		return ErrRecovery
	}
	n, writeErr := f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(b) || syncErr != nil || closeErr != nil || !sameActivationDirectory(d, gid) {
		return ErrRecovery
	}
	if unix.Renameat2(int(d.Fd()), ".activation-next", int(d.Fd()), name, unix.RENAME_NOREPLACE) != nil || d.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func activationRecord(d *os.File, name string, uid, gid uint32, mode os.FileMode) ([]byte, error) {
	fd, err := unix.Openat(int(d.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "activation-record")
	defer f.Close()
	if !regular(f, uid, mode, connectedactivation.MaxBytes) {
		return nil, ErrUnsafe
	}
	i, err := f.Stat()
	if err != nil {
		return nil, ErrUnsafe
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Gid != gid || i.Size() < 1 {
		return nil, ErrUnsafe
	}
	b, err := io.ReadAll(io.LimitReader(f, connectedactivation.MaxBytes+1))
	if err != nil || len(b) > connectedactivation.MaxBytes {
		return nil, ErrUnsafe
	}
	return b, nil
}

func responseDirectory(c connectedprofile.Config) (*os.File, error) {
	parent, err := rootDirectory(connectedprofile.StateDirectory)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer parent.Close()
	return privateDirectoryAt(parent, "uploader-status", c.UploaderUID, c.UploaderGID)
}

func clearActivationResponse(c connectedprofile.Config) error {
	d, err := responseDirectory(c)
	if err != nil {
		return err
	}
	defer d.Close()
	b, err := activationRecord(d, "activation-response.json", c.UploaderUID, c.UploaderGID, 0600)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return ErrUnsafe
	}
	if _, err := connectedactivation.DecodeResponse(b); err != nil {
		return ErrUnsafe
	}
	if unix.Unlinkat(int(d.Fd()), "activation-response.json", 0) != nil || d.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func readActivationResponse(c connectedprofile.Config) (connectedactivation.Response, error) {
	d, err := responseDirectory(c)
	if err != nil {
		return connectedactivation.Response{}, err
	}
	defer d.Close()
	b, err := activationRecord(d, "activation-response.json", c.UploaderUID, c.UploaderGID, 0600)
	if err != nil {
		return connectedactivation.Response{}, err
	}
	return connectedactivation.DecodeResponse(b)
}
