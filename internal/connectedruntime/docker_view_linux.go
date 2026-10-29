//go:build linux

package connectedruntime

import (
	"io"
	"os"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// CheckRichProcessesDockerView preserves both older private-state denials. The
// collector may inspect processes and the bounded frame, never either socket.
func CheckRichProcessesDockerView(collector bool) error {
	if CheckRichProcessesView(collector) != nil {
		return ErrUnsafe
	}
	if !collector {
		return nil
	}
	return checkDockerCollectorSecrets(func(path string) (io.Closer, error) { return os.Open(path) })
}

func checkDockerCollectorSecrets(open func(string) (io.Closer, error)) error {
	for _, path := range []string{"/run/docker.sock", "/var/run/docker.sock", "/engine/docker.sock", connectedprofile.RichProcessesDockerReaderRoot, connectedprofile.RichProcessesDockerReaderStatusDirectory, "/run/home-lab-observer-connected-rich/activation", "/run/home-lab-observer-connected-rich/reader-activation"} {
		f, err := open(path)
		if err == nil {
			f.Close()
			return ErrUnsafe
		}
		if !os.IsPermission(err) && !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	return nil
}

// CheckRichProcessesDockerReaderView checks only the fixed minimal filesystem;
// the helper separately pins the approved socket and checks network primitives.
func CheckRichProcessesDockerReaderView(c connectedprofile.RichProcessesDockerConfig) error {
	if connectedprofile.CheckRichProcessesDockerReaderIdentity(c) != nil {
		return ErrUnsafe
	}
	return checkDockerReaderView(c, readViewDirectoryLimit, unix.Access, os.Lstat)
}

func checkDockerReaderView(c connectedprofile.RichProcessesDockerConfig, read func(string, int) (viewDirectory, error), access func(string, uint32) error, lstat func(string) (os.FileInfo, error)) error {
	if c.Validate() != nil || checkUploaderView(func(path string) (viewDirectory, error) { return read(path, 3) }, access, lstat) != nil {
		return ErrUnsafe
	}
	specs := []scaffoldDirectory{
		{"", 0755, readerRootChildren},
		{"bin", 0755, []string{"observer-connected-rich-docker-reader"}},
		{"etc", 0755, []string{"home-lab-observer-connected-rich"}},
		{"etc/home-lab-observer-connected-rich", 0755, []string{"collection-policy.json", "installed.json"}},
		{"engine", 0755, []string{"docker.sock"}},
		{"state", 0755, []string{"status"}},
	}
	for _, spec := range specs {
		path := "/" + spec.name
		d, err := read(path, len(spec.children)+1)
		if err != nil || !safeScaffold(d, spec, 0, 0, false) {
			return ErrUnsafe
		}
		if err := access(path, unix.W_OK); err != unix.EACCES && err != unix.EROFS {
			return ErrUnsafe
		}
	}
	for _, spec := range uploaderScaffold[7:] {
		d, err := read("/"+spec.name, len(spec.children)+1)
		if err != nil || !safeScaffold(d, spec, 0, 0, false) {
			return ErrUnsafe
		}
	}
	activation, err := read("/activation", 4)
	if err != nil || activation.stat.Mode != unix.S_IFDIR|0750 || activation.stat.Uid != 0 || activation.stat.Gid != c.FrameGID || !activation.noACL || activation.denied || activation.filesystem == unix.PROC_SUPER_MAGIC || activation.filesystem == unix.SYSFS_MAGIC {
		return ErrUnsafe
	}
	if err := access("/activation", unix.W_OK); err != unix.EACCES && err != unix.EROFS {
		return ErrUnsafe
	}
	for _, writable := range []struct {
		path string
		mode uint32
	}{{"/frame", 0750}, {"/state/status", 0700}} {
		d, err := read(writable.path, 4)
		if err != nil || d.stat.Mode != unix.S_IFDIR|writable.mode || d.stat.Uid != c.ReaderUID || d.stat.Gid != c.FrameGID || !d.noACL || d.denied || d.filesystem == unix.PROC_SUPER_MAGIC || d.filesystem == unix.SYSFS_MAGIC || access(writable.path, unix.W_OK) != nil {
			return ErrUnsafe
		}
	}
	return nil
}
