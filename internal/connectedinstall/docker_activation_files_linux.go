//go:build linux

package connectedinstall

import (
	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"golang.org/x/sys/unix"
	"os"
)

const dockerReaderActivationParent = "/run/home-lab-observer-connected-rich"
const dockerReaderActivationPath = dockerReaderActivationParent + "/reader-activation"

// Caller holds the install lease and has proved every fixed Docker worker
// stopped. Unknown staging/foreign state is recovery evidence, never adopted.
func pinDockerReaderActivation(gid uint32) (*os.File, error) {
	if gid == 0 || gid == ^uint32(0) {
		return nil, ErrUnsafe
	}
	if _, err := os.Lstat(dockerReaderActivationParent); os.IsNotExist(err) {
		if createActivationDirectoryForPair(dockerReaderActivationParent, 0, 0700, richProcessesDockerPair) != nil {
			return nil, ErrRecovery
		}
	} else if err != nil {
		return nil, ErrUnsafe
	}
	p, err := rootDirectory(dockerReaderActivationParent)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer p.Close()
	if !activationDirectory(p, 0, 0700) {
		return nil, ErrUnsafe
	}
	if _, err := os.Lstat(dockerReaderActivationPath); os.IsNotExist(err) {
		if createDirectoryAt(p, directorySpec{parent: dockerReaderActivationParent, name: "reader-activation", gid: gid, mode: 0750}, nil) != nil {
			return nil, ErrRecovery
		}
	} else if err != nil {
		return nil, ErrUnsafe
	}
	d, err := rootDirectory(dockerReaderActivationPath)
	if err != nil {
		return nil, ErrUnsafe
	}
	if !activationDirectory(d, gid, 0750) {
		d.Close()
		return nil, ErrUnsafe
	}
	if err := clearPinnedActivationRecords(d, gid); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}
func sameDockerReaderActivation(d *os.File, gid uint32) bool {
	current, err := rootDirectory(dockerReaderActivationPath)
	if err != nil {
		return false
	}
	defer current.Close()
	a, e1 := d.Stat()
	b, e2 := current.Stat()
	return e1 == nil && e2 == nil && os.SameFile(a, b) && activationDirectory(current, gid, 0750)
}
func publishDockerReaderActivation(d *os.File, name string, b []byte, gid uint32) error {
	return publishPinnedActivation(d, name, b, gid, func() bool { return sameDockerReaderActivation(d, gid) })
}
func dockerReaderStatusDirectory(c connectedprofile.RichProcessesDockerConfig) (*os.File, error) {
	p, err := rootDirectory(connectedprofile.RichProcessesDockerStateDirectory)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer p.Close()
	return privateDirectoryAt(p, "reader-status", c.ReaderUID, c.FrameGID)
}
func clearDockerReaderResponse(c connectedprofile.RichProcessesDockerConfig) error {
	d, err := dockerReaderStatusDirectory(c)
	if err != nil {
		return err
	}
	defer d.Close()
	b, err := activationRecord(d, "activation-response.json", c.ReaderUID, c.FrameGID, 0600)
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
func readDockerReaderResponse(c connectedprofile.RichProcessesDockerConfig) (connectedactivation.Response, error) {
	d, err := dockerReaderStatusDirectory(c)
	if err != nil {
		return connectedactivation.Response{}, err
	}
	defer d.Close()
	b, err := activationRecord(d, "activation-response.json", c.ReaderUID, c.FrameGID, 0600)
	if err != nil {
		return connectedactivation.Response{}, err
	}
	return connectedactivation.DecodeResponse(b)
}
