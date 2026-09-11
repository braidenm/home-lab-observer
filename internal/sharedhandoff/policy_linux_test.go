//go:build linux

package sharedhandoff

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPolicyRejectsAmbiguousPrincipals(t *testing.T) {
	if !validPolicy(fixturePolicy) {
		t.Fatal("valid policy rejected")
	}
	for _, mutate := range []func(*Policy){
		func(p *Policy) { p.CollectorUID = 0 }, func(p *Policy) { p.SharedGID = 0 }, func(p *Policy) { p.UploaderUID = 0 },
		func(p *Policy) { p.UploaderUID = p.CollectorUID }, func(p *Policy) { p.ServerID = "private-canary" },
	} {
		p := fixturePolicy
		mutate(&p)
		if validPolicy(p) {
			t.Fatal("invalid policy accepted")
		}
		if identity(p, true) != ErrUnsafe {
			t.Fatal("invalid identity accepted")
		}
	}
}

func TestRealEffectiveSavedGroupsHaveNoLatentAuthority(t *testing.T) {
	if !consistentGroups(61012, 61012, 61012) {
		t.Fatal("ordinary primary group rejected")
	}
	for _, ids := range [][3]int{{61012, 61012, 0}, {0, 61012, 61012}, {61012, 61011, 61012}, {0, 0, 0}, {-1, -1, -1}} {
		if consistentGroups(ids[0], ids[1], ids[2]) {
			t.Fatal("latent group authority accepted")
		}
	}
}

func TestHandleRejectsUnsafeModesLinksAndACL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owned-file")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Chmod(0o640); err != nil {
		t.Fatal(err)
	}
	p := Policy{CollectorUID: uint32(os.Geteuid()), SharedGID: uint32(os.Getegid())}
	if err := checkHandle(f, p, false, 0o640); err != nil {
		t.Fatal("ordinary handle rejected")
	}
	if err := f.Chmod(0o660); err != nil {
		t.Fatal(err)
	}
	if checkHandle(f, p, false, 0o640) != ErrUnsafe {
		t.Fatal("writable group accepted")
	}
	if err := f.Chmod(0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "owned-link")
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	if checkHandle(f, p, false, 0o640) != ErrUnsafe {
		t.Fatal("multiple links accepted")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := unix.Fsetxattr(int(f.Fd()), "system.posix_acl_access", fixtureACL(false), 0); err != nil {
		t.Fatal("owned ACL fixture unsupported")
	}
	if checkHandle(f, p, false, 0o640) != ErrUnsafe {
		t.Fatal("named ACL accepted")
	}
	if err := unix.Fremovexattr(int(f.Fd()), "system.posix_acl_access"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if checkHandle(f, p, false, 0o640) != ErrUnavailable {
		t.Fatal("unlinked handle accepted")
	}
	f.Close()
	if noACL(f, false) != ErrUnsafe {
		t.Fatal("closed ACL handle accepted")
	}
}

func TestBoundedFixtureOutput(t *testing.T) {
	w := &cappedOutput{}
	if _, err := w.Write(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte{1}); err == nil || !w.overflow {
		t.Fatal("fixture output not bounded")
	}
}
