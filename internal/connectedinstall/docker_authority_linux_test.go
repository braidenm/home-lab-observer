//go:build linux

package connectedinstall

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func dockerAuthorityConfig(t *testing.T) connectedprofile.RichProcessesDockerConfig {
	t.Helper()
	p, err := connectedprofile.NewRichProcessesDockerPolicy("srv_"+strings.Repeat("a", 32), "agent_"+strings.Repeat("b", 32), 60101, 60102, 60103, 60104, 60105, 60106, 999)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := connectedprofile.DigestRichProcessesDockerPolicy(p)
	return connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: p.Profile, ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, ReaderUID: p.ReaderUID, FrameGID: p.FrameGID, SocketGID: p.SocketGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}

func TestDockerReaderNSSIsExclusiveWithoutPersistentSocketMembership(t *testing.T) {
	c := dockerAuthorityConfig(t)
	passwd := "root:x:0:0:root:/root:/bin/bash\nhlo-rich-docker-reader:x:60105:60106::/nonexistent:/usr/sbin/nologin\n"
	groups := "root:x:0:\ndocker:x:999:ordinary-owner\nhlo-rich-docker-frame:x:60106:\n"
	for _, change := range []string{"valid", "uid-alias", "gid-alias", "primary-outsider", "shell", "home", "password", "reader-membership", "frame-membership", "duplicate-reader", "duplicate-frame", "missing-reader", "missing-frame", "group-alias", "remote-nss", "zero-reader"} {
		t.Run(change, func(t *testing.T) {
			p, g, n, config := passwd, groups, fixtureNSS, c
			switch change {
			case "uid-alias":
				p += "alias:x:60105:65534::/nonexistent:/usr/sbin/nologin\n"
			case "gid-alias":
				p = strings.Replace(p, "60105:60106:", "60105:999:", 1)
			case "primary-outsider":
				p += "outsider:x:61000:60106::/nonexistent:/usr/sbin/nologin\n"
			case "shell":
				p = strings.Replace(p, "/usr/sbin/nologin", "/bin/bash", 1)
			case "home":
				p = strings.Replace(p, "/nonexistent", "/root", 1)
			case "password":
				p = strings.Replace(p, "reader:x:", "reader:!:", 1)
			case "reader-membership":
				g = strings.Replace(g, "ordinary-owner", "ordinary-owner,hlo-rich-docker-reader", 1)
			case "frame-membership":
				g = strings.Replace(g, "frame:x:60106:", "frame:x:60106:hlo-rich-docker-collector", 1)
			case "duplicate-reader":
				p += strings.Split(p, "\n")[1] + "\n"
			case "duplicate-frame":
				g += "hlo-rich-docker-frame:x:60106:\n"
			case "missing-reader":
				p = strings.Split(p, "\n")[0] + "\n"
			case "missing-frame":
				g = "root:x:0:\n"
			case "group-alias":
				g += "alias:x:60106:\n"
			case "remote-nss":
				n = strings.Replace(n, "passwd: files systemd", "passwd: sss files", 1)
			case "zero-reader":
				config.ReaderUID = 0
			}
			_, err := parseDockerReaderPrincipals([]byte(n), []byte(p), []byte(g), config)
			if (err == nil) != (change == "valid") {
				t.Fatal("Docker reader authority boundary mismatch")
			}
		})
	}
}

func TestDockerSocketMetadataRequiresRootfulExactNode(t *testing.T) {
	valid := unix.Stat_t{Mode: unix.S_IFSOCK | 0660, Uid: 0, Gid: 999, Nlink: 1, Dev: 1, Ino: 2}
	for _, change := range []string{"valid", "mode", "world", "uid", "gid", "root-group", "link", "regular", "symlink", "sticky"} {
		s := valid
		switch change {
		case "mode":
			s.Mode = unix.S_IFSOCK | 0600
		case "world":
			s.Mode |= 0006
		case "uid":
			s.Uid = 1000
		case "gid":
			s.Gid = 998
		case "root-group":
			s.Gid = 0
		case "link":
			s.Nlink = 2
		case "regular":
			s.Mode = unix.S_IFREG | 0660
		case "symlink":
			s.Mode = unix.S_IFLNK | 0660
		case "sticky":
			s.Mode |= unix.S_ISVTX
		}
		if validDockerSocket(s, 999) != (change == "valid") {
			t.Fatal("socket metadata boundary mismatch")
		}
	}
	if validDockerSocket(valid, 0) {
		t.Fatal("zero approved socket group accepted")
	}
}

func TestRootDockerSocketWitnessPinsNodeAndRefusesReplacement(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("fresh temporary root socket fixture")
	}
	dir := t.TempDir()
	parent, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	path := filepath.Join(dir, "docker.sock")
	listen := func() *net.UnixListener {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		l.SetUnlinkOnClose(false)
		if os.Chown(path, 0, 999) != nil || os.Chmod(path, 0660) != nil {
			t.Fatal("fixture node metadata failed")
		}
		return l
	}
	listener := listen()
	defer listener.Close()
	witness, err := captureDockerSocketAt(parent, 999)
	if err != nil {
		t.Fatal("approved socket refused", err)
	}
	defer witness.Close()
	if !sameDockerSocketAt(parent, witness) {
		t.Fatal("stable pinned socket refused")
	}
	discovered, err := openDockerSocketAt(parent, 0, true)
	if err != nil || discovered.gid != 999 || discovered.dev != witness.dev || discovered.ino != witness.ino {
		t.Fatal("fresh discovery did not preserve approved node")
	}
	discovered.Close()
	if _, err := captureDockerSocketAt(parent, 0); err != ErrUnsafe {
		t.Fatal("Start admitted unspecified socket approval")
	}
	if _, err := captureDockerSocketAt(parent, 998); err != ErrUnsafe {
		t.Fatal("unapproved group adopted")
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if sameDockerSocketAt(parent, witness) {
		t.Fatal("socket permission drift accepted")
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	link := path + ".link"
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if sameDockerSocketAt(parent, witness) {
		t.Fatal("multiply linked socket accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	acl := make([]byte, 44)
	binary.LittleEndian.PutUint32(acl, 2)
	for i, tag := range []uint16{1, 2, 4, 16, 32} {
		entry := acl[4+i*8 : 12+i*8]
		binary.LittleEndian.PutUint16(entry, tag)
		permission := uint16(6)
		if tag == 32 {
			permission = 0
		}
		binary.LittleEndian.PutUint16(entry[2:], permission)
		id := ^uint32(0)
		if tag == 2 {
			id = 60111
		}
		binary.LittleEndian.PutUint32(entry[4:], id)
	}
	if err := unix.Setxattr(path, "system.posix_acl_access", acl, 0); err != nil {
		t.Fatal("temporary ACL fixture unavailable")
	}
	if sameDockerSocketAt(parent, witness) {
		t.Fatal("socket ACL grant accepted")
	}
	if err := unix.Removexattr(path, "system.posix_acl_access"); err != nil {
		t.Fatal(err)
	}
	if !sameDockerSocketAt(parent, witness) {
		t.Fatal("restored exact socket refused")
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	replacement := listen()
	defer replacement.Close()
	if sameDockerSocketAt(parent, witness) {
		t.Fatal("socket replacement silently adopted")
	}
	var old unix.Stat_t
	if unix.Fstat(int(witness.handle.Fd()), &old) != nil || uint64(old.Ino) != witness.ino {
		t.Fatal("old socket inode was not pinned")
	}
	if err := witness.Close(); err != nil {
		t.Fatal(err)
	}
	if sameDockerSocketAt(parent, witness) {
		t.Fatal("closed socket witness accepted")
	}
}
