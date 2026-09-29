//go:build linux

package connectedinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"golang.org/x/sys/unix"
)

// This opt-in root fixture touches only a fresh owned temporary tree.
func TestRootMissingProviderRequiresTrustedAncestor(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("fresh temporary root fixture")
	}
	for _, scenario := range []string{"trusted", "missing-intermediate", "writable", "symlink", "anchor-writable"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			if scenario == "anchor-writable" {
				if err := os.Chmod(base, 0777); err != nil {
					t.Fatal(err)
				}
			}
			parent := filepath.Join(base, "parent")
			if scenario == "symlink" {
				if err := os.Symlink("missing", parent); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario == "writable" {
					if err := os.Chmod(parent, 0777); err != nil {
						t.Fatal(err)
					}
				}
			}
			fd, err := unix.Open(base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			parts := []string{"parent", "userdb"}
			if scenario == "missing-intermediate" {
				parts = []string{"parent", "absent", "userdb"}
			}
			d, err := trustedDescendant(fd, parts, true)
			if d != nil {
				d.Close()
				t.Fatal("missing descendant returned a directory")
			}
			if scenario == "trusted" && err != nil {
				t.Fatal("trusted absence refused", err)
			}
			if scenario != "trusted" && err != ErrUnsafe {
				t.Fatal("unsafe ancestor accepted")
			}
		})
	}
}

const fixtureNSS = "passwd: files systemd\ngroup: files systemd\nshadow: files systemd\ngshadow: files systemd\nhosts: files dns\n"
const fixturePasswd = "root:x:0:0:root:/root:/bin/bash\nhlo-connected-collector:x:60101:60103::/nonexistent:/usr/sbin/nologin\nhlo-connected-uploader:x:60102:60102::/nonexistent:/usr/sbin/nologin\n"
const fixtureGroups = "root:x:0:\nhlo-connected-read:x:60103:hlo-connected-uploader\nhlo-connected-uploader:x:60102:\n"

func principalFixture() connectedprofile.Config {
	return connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}

func TestSupportedLocalFirstNSSIsExplicit(t *testing.T) {
	if !supportedNSS([]byte(fixtureNSS)) || !supportedNSS([]byte(strings.ReplaceAll(fixtureNSS, "files systemd", "files"))) {
		t.Fatal("supported local-first sources refused")
	}
	for _, bad := range []string{
		strings.Replace(fixtureNSS, "passwd: files systemd", "passwd: systemd files", 1),
		strings.Replace(fixtureNSS, "group: files systemd", "group: files [SUCCESS=merge] systemd", 1),
		strings.Replace(fixtureNSS, "shadow: files systemd", "shadow: files sss", 1),
		fixtureNSS + "initgroups: files systemd\n", fixtureNSS + "passwd: files\n",
		strings.Replace(fixtureNSS, "gshadow: files systemd\n", "", 1),
	} {
		if supportedNSS([]byte(bad)) {
			t.Fatal("custom or ambiguous identity authority accepted")
		}
	}
}

func TestDedicatedNamesNumbersAndMemberships(t *testing.T) {
	c := principalFixture()
	if _, err := parsePrincipals([]byte(fixtureNSS), []byte(fixturePasswd), []byte(fixtureGroups), c); err != nil {
		t.Fatal("valid local roles refused", err)
	}
	for _, tc := range []struct{ name, passwd, groups string }{
		{"uid-alias", fixturePasswd + "alias:x:60101:65534::/nonexistent:/usr/sbin/nologin\n", fixtureGroups},
		{"primary-group-outsider", fixturePasswd + "outsider:x:61000:60103::/nonexistent:/usr/sbin/nologin\n", fixtureGroups},
		{"collector-wrong-primary", strings.Replace(fixturePasswd, "60101:60103:", "60101:65534:", 1), fixtureGroups},
		{"uploader-wrong-primary", strings.Replace(fixturePasswd, "60102:60102:", "60102:60103:", 1), fixtureGroups},
		{"login-shell", strings.Replace(fixturePasswd, "/usr/sbin/nologin", "/bin/bash", 1), fixtureGroups},
		{"home", strings.Replace(fixturePasswd, "/nonexistent", "/home/shared", 1), fixtureGroups},
		{"group-alias", fixturePasswd, fixtureGroups + "alias:x:60103:\n"},
		{"extra-supplemental", fixturePasswd, fixtureGroups + "sudo:x:27:hlo-connected-uploader\n"},
		{"extra-member", fixturePasswd, strings.Replace(fixtureGroups, "60103:hlo-connected-uploader", "60103:hlo-connected-uploader,outsider", 1)},
		{"missing-shared-membership", fixturePasswd, strings.Replace(fixtureGroups, "60103:hlo-connected-uploader", "60103:", 1)},
		{"duplicate-name", fixturePasswd + "hlo-connected-uploader:x:60102:60102::/nonexistent:/usr/sbin/nologin\n", fixtureGroups},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parsePrincipals([]byte(fixtureNSS), []byte(tc.passwd), []byte(tc.groups), c); err != ErrUnsafe {
				t.Fatal("non-exclusive principal accepted")
			}
		})
	}
}

func TestEffectivePrincipalQueriesAreClosed(t *testing.T) {
	if !exactInitgroups([]byte(uploaderName+" 60103\n"), uploaderName, []uint32{60103}) || !exactInitgroups([]byte(collectorName+"\n"), collectorName, nil) {
		t.Fatal("effective group mapping refused")
	}
	for _, bad := range []string{uploaderName + " 60102 60103\n", uploaderName + " 60103 60103\n", uploaderName + " 60102\n", uploaderName + "\n", uploaderName + " 60103\nextra\n"} {
		if exactInitgroups([]byte(bad), uploaderName, []uint32{60103}) {
			t.Fatal("expanded effective groups accepted")
		}
	}
	if exactInitgroups([]byte(collectorName+" 60103\n"), collectorName, nil) {
		t.Fatal("unexpected collector supplementary group accepted")
	}
	if !lockedStatus([]byte(uploaderName+" L 2026-09-17 0 99999 7 -1\n"), uploaderName) {
		t.Fatal("locked status refused")
	}
	if lockedStatus([]byte(uploaderName+" NP 2026-09-17 0 99999 7 -1\n"), uploaderName) {
		t.Fatal("unlocked account accepted")
	}
	if !lockedGroup([]byte(sharedName+":!::"+uploaderName+"\n"), sharedName, uploaderName) {
		t.Fatal("locked dedicated group refused")
	}
	for _, bad := range []string{sharedName + ":::" + uploaderName + "\n", sharedName + ":!:outsider:" + uploaderName + "\n", sharedName + ":!::outsider\n"} {
		if lockedGroup([]byte(bad), sharedName, uploaderName) {
			t.Fatal("group admission authority accepted")
		}
	}
}
