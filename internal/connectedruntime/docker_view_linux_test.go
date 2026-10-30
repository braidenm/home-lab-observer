//go:build linux

package connectedruntime

import (
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func dockerViewConfig(t *testing.T) connectedprofile.RichProcessesDockerConfig {
	t.Helper()
	p, err := connectedprofile.NewRichProcessesDockerPolicy("srv_"+strings.Repeat("a", 32), "agent_"+strings.Repeat("b", 32), 60101, 60102, 60103, 60104, 60105, 60106, 999)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := connectedprofile.DigestRichProcessesDockerPolicy(p)
	return connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: p.Profile, ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, ReaderUID: p.ReaderUID, FrameGID: p.FrameGID, SocketGID: p.SocketGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}

func TestDockerReaderViewRejectsExtraAuthorityAndPermissionDrift(t *testing.T) {
	c := dockerViewConfig(t)
	for _, change := range []string{"valid", "root-extra", "credential", "network-config", "procfs", "sysfs", "sys-extra", "sys-mode", "frame-owner", "frame-setgid", "frame-group", "frame-acl", "frame-readonly", "status-group", "root-writable", "run-credential", "home", "read-error", "activation-owner", "activation-group", "activation-mode", "activation-acl", "activation-writable"} {
		t.Run(change, func(t *testing.T) {
			dirs := map[string]viewDirectory{}
			add := func(path string, mode, uid, gid uint32, children []string, denied bool) {
				dirs[path] = viewDirectory{stat: unix.Stat_t{Mode: unix.S_IFDIR | mode, Uid: uid, Gid: gid}, filesystem: unix.EXT4_SUPER_MAGIC, noACL: true, entries: slices.Clone(children), denied: denied}
			}
			for _, spec := range uploaderScaffold {
				add("/"+spec.name, spec.mode, 0, 0, spec.children, spec.name == "root")
			}
			add("/", 0755, 0, 0, readerRootChildren, false)
			add("/bin", 0755, 0, 0, []string{"observer-connected-rich-docker-reader"}, false)
			add("/etc", 0755, 0, 0, []string{"home-lab-observer-connected-rich"}, false)
			add("/etc/home-lab-observer-connected-rich", 0755, 0, 0, []string{"collection-policy.json", "installed.json"}, false)
			add("/engine", 0755, 0, 0, []string{"docker.sock"}, false)
			add("/state", 0755, 0, 0, []string{"status"}, false)
			add("/frame", 0750, c.ReaderUID, c.FrameGID, nil, false)
			add("/state/status", 0700, c.ReaderUID, c.FrameGID, nil, false)
			add("/activation", 0750, 0, c.FrameGID, []string{"request.json"}, false)
			mutate := func(path string, fn func(*viewDirectory)) { d := dirs[path]; fn(&d); dirs[path] = d }
			switch change {
			case "root-extra":
				mutate("/", func(d *viewDirectory) { d.entries = append(d.entries, "handoff") })
			case "credential":
				mutate("/etc/home-lab-observer-connected-rich", func(d *viewDirectory) { d.entries = append(d.entries, "credentials") })
			case "network-config":
				mutate("/etc", func(d *viewDirectory) { d.entries = append(d.entries, "resolv.conf") })
			case "procfs":
				mutate("/proc", func(d *viewDirectory) { d.filesystem = unix.PROC_SUPER_MAGIC })
			case "sysfs":
				mutate("/sys", func(d *viewDirectory) { d.filesystem = unix.SYSFS_MAGIC })
			case "sys-extra":
				mutate("/sys", func(d *viewDirectory) { d.entries = []string{"devices"} })
			case "sys-mode":
				mutate("/sys", func(d *viewDirectory) { d.stat.Mode = unix.S_IFDIR | 0755 })
			case "frame-owner":
				mutate("/frame", func(d *viewDirectory) { d.stat.Uid++ })
			case "frame-setgid":
				mutate("/frame", func(d *viewDirectory) { d.stat.Mode |= unix.S_ISGID })
			case "frame-group":
				mutate("/frame", func(d *viewDirectory) { d.stat.Gid++ })
			case "frame-acl":
				mutate("/frame", func(d *viewDirectory) { d.noACL = false })
			case "status-group":
				mutate("/state/status", func(d *viewDirectory) { d.stat.Gid++ })
			case "run-credential":
				mutate("/run", func(d *viewDirectory) { d.entries = append(d.entries, "credentials") })
			case "activation-owner":
				mutate("/activation", func(d *viewDirectory) { d.stat.Uid = c.ReaderUID })
			case "activation-group":
				mutate("/activation", func(d *viewDirectory) { d.stat.Gid = c.SocketGID })
			case "activation-mode":
				mutate("/activation", func(d *viewDirectory) { d.stat.Mode = unix.S_IFDIR | 0770 })
			case "activation-acl":
				mutate("/activation", func(d *viewDirectory) { d.noACL = false })
			}
			read := func(path string, _ int) (viewDirectory, error) {
				if change == "read-error" {
					return viewDirectory{}, unix.EIO
				}
				d, ok := dirs[path]
				if !ok {
					t.Fatal("unexpected view path", path)
				}
				return d, nil
			}
			access := func(path string, mode uint32) error {
				if path == "/activation" && change == "activation-writable" {
					return nil
				}
				if path == "/frame" || path == "/state/status" {
					if change == "frame-readonly" {
						return unix.EROFS
					}
					return nil
				}
				if path == "/" && change == "root-writable" {
					return nil
				}
				return unix.EACCES
			}
			lstat := func(string) (os.FileInfo, error) {
				if change == "home" {
					return nil, nil
				}
				return nil, os.ErrNotExist
			}
			if (checkDockerReaderView(c, read, access, lstat) == nil) != (change == "valid") {
				t.Fatal("reader view boundary mismatch")
			}
		})
	}
}
