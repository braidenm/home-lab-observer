//go:build linux

package connectedinstall

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

const actionStateRoot = "/var/lib/home-lab-observer-actions"
const actionRunRoot = "/run/home-lab-observer-actions"

// Action service views use root-owned ancestors. Only the three runtime
// leaves (credential store, journal, listener) are writable by a service.
func actionDirectories(i actionprofile.Identity) []directorySpec {
	dirs := []directorySpec{
		{parent: "/etc", name: "home-lab-observer-actions", gid: i.IPCGID, mode: 0750},
		{parent: "/var/lib", name: "home-lab-observer-actions", mode: 0755},
		{parent: "/run", name: "home-lab-observer-actions", mode: 0755},
		{parent: actionStateRoot, name: "worker", uid: i.WorkerUID, gid: i.WorkerGID, mode: 0700},
		{parent: actionStateRoot, name: "journal", uid: i.BrokerUID, gid: i.BrokerGID, mode: 0700},
		{parent: actionRunRoot, name: "worker", uid: i.BrokerUID, gid: i.IPCGID, mode: 0750},
	}
	for _, role := range []string{"worker", "broker"} {
		name := role + "-root"
		root := actionStateRoot + "/" + name
		dirs = append(dirs, directorySpec{parent: actionStateRoot, name: name, mode: 0755})
		for _, path := range []struct{ parent, name string }{
			{"", "bin"}, {"", "etc"}, {"", "proc"}, {"", "run"}, {"", "var"},
			{"/etc", "home-lab-observer-actions"}, {"/run", "home-lab-observer-actions"},
			{"/var", "lib"}, {"/var/lib", "home-lab-observer-actions"},
		} {
			dirs = append(dirs, directorySpec{parent: root + path.parent, name: path.name, mode: 0755})
		}
		if role == "worker" {
			dirs = append(dirs, directorySpec{parent: root + "/etc", name: "ssl", mode: 0755},
				directorySpec{parent: root + "/etc/ssl", name: "certs", mode: 0755})
		} else {
			dirs = append(dirs, directorySpec{parent: root, name: "engine", mode: 0755})
		}
	}
	return dirs
}

func createActionDirectories(ctx context.Context, i actionprofile.Identity) error {
	if ctx == nil || ctx.Err() != nil || i.WorkerUID == 0 || i.BrokerUID == 0 ||
		i.WorkerGID == 0 || i.BrokerGID == 0 || i.IPCGID == 0 || i.WorkerUID == i.BrokerUID ||
		i.WorkerGID == i.BrokerGID || i.WorkerGID == i.IPCGID || i.BrokerGID == i.IPCGID {
		return ErrUnsafe
	}
	for _, spec := range actionDirectories(i) {
		if ctx.Err() != nil {
			return ErrRecovery
		}
		parent, err := rootDirectory(spec.parent)
		if err != nil {
			return ErrRecovery
		}
		createErr := createDirectoryAt(parent, spec, nil)
		closeErr := parent.Close()
		if createErr != nil || closeErr != nil {
			return ErrRecovery
		}
	}
	return nil
}
