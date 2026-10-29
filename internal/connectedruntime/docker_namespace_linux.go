//go:build linux

package connectedruntime

import (
	"os"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var readerRootChildren = []string{"activation", "bin", "dev", "engine", "etc", "frame", "proc", "root", "run", "state", "sys", "usr", "var"}

func CheckInstalledRichProcessesDockerUploaderScaffold() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	return checkPhysicalScaffold(connectedprofile.RichProcessesDockerStateDirectory+"/uploader-root", 0, 0)
}

func CheckInstalledRichProcessesDockerReaderScaffold() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	return checkPhysicalScaffoldChildren(connectedprofile.RichProcessesDockerReaderRoot, 0, 0, readerRootChildren, false)
}

func CheckRichProcessesDockerUploaderNamespace(pid int, root string, sources map[string]string) error {
	return checkDockerNamespace(pid, root, sources, false)
}

func CheckRichProcessesDockerReaderNamespace(pid int, root string, sources map[string]string) error {
	return checkDockerNamespace(pid, root, sources, true)
}

func checkDockerNamespace(pid int, root string, sources map[string]string, reader bool) error {
	expected := connectedprofile.RichProcessesDockerStateDirectory + "/uploader-root"
	if reader {
		expected = connectedprofile.RichProcessesDockerReaderRoot
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 || pid < 2 || root != expected {
		return ErrUnsafe
	}
	return checkNamespaceView(pid, root, sources, func(actual, physical func(string) (unix.Stat_t, error)) error {
		return checkNamespaceBindingsTargets(root, sources, dockerNamespaceTargets(reader), actual, physical)
	})
}

func dockerNamespaceTargets(reader bool) []string {
	if reader {
		return []string{"/bin/observer-connected-rich-docker-reader", connectedprofile.RichProcessesDockerConfigPath, connectedprofile.RichProcessesDockerPolicyPath, "/activation", "/frame", "/state/status", "/engine/docker.sock"}
	}
	return []string{"/bin/observer-connected-rich-docker-uploader", connectedprofile.RichProcessesDockerConfigPath, connectedprofile.RichProcessesDockerPolicyPath, "/handoff", "/activation", "/state/ledger", "/state/status"}
}
