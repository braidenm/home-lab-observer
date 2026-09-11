//go:build windows

package handoff

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only called for the new t.TempDir fixture, never existing owner paths. Hosted
// elevated runners may default newly created objects to the Administrators SID.
func setFixtureDirectoryOwner(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal("fixture user lookup failed")
	}
	if windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION,
		user.User.Sid, nil, nil, nil) != nil {
		t.Fatal("fixture owner setup failed")
	}
}

func TestCreatedWindowsFilesHaveExactCurrentOwner(t *testing.T) {
	dir := privateDirectory(t)
	s := openStore(t, dir, true)
	raw, id := sample()
	if s.Publish(raw, id) != nil {
		t.Fatal("new-file initialization or publish failed")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal("fixture user lookup failed")
	}
	for _, name := range []string{lockName, latestName} {
		sd, err := windows.GetNamedSecurityInfo(filepath.Join(dir, name), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal("created owner query failed")
		}
		owner, _, err := sd.Owner()
		if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
			t.Fatal("new file retained token group ownership")
		}
	}
}

func TestExistingDefaultGroupOwnedFileIsNotRepaired(t *testing.T) {
	dir := privateDirectory(t)
	path := filepath.Join(dir, lockName)
	if os.WriteFile(path, nil, 0o600) != nil {
		t.Fatal("fixture creation failed")
	}
	before, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal("fixture descriptor query failed")
	}
	owner, _, err := before.Owner()
	user, userErr := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || userErr != nil || owner == nil {
		t.Fatal("fixture owner query failed")
	}
	if owner.Equals(user.User.Sid) {
		t.Skip("token defaults to current user; elevated group-owner fixture not present")
	}
	if s, err := Open(dir, true); err != ErrUnsafe {
		if s != nil {
			s.Close()
		}
		t.Fatal("existing group-owned lock accepted")
	}
	after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || before.String() != after.String() {
		t.Fatal("existing group-owned file was modified")
	}
}

func TestPrivateACLBytesBounds(t *testing.T) {
	owner, err := windows.StringToSid("S-1-5-21-100-200-300-1000")
	if err != nil {
		t.Fatal("synthetic SID failed")
	}
	sidBytes := unsafe.Slice((*byte)(unsafe.Pointer(owner)), owner.Len())
	valid := make([]byte, 16+len(sidBytes))
	valid[0] = 2
	binary.LittleEndian.PutUint16(valid[2:4], uint16(len(valid)))
	binary.LittleEndian.PutUint16(valid[4:6], 1)
	binary.LittleEndian.PutUint16(valid[10:12], uint16(8+len(sidBytes)))
	copy(valid[16:], sidBytes)
	if privateACLBytes(valid, owner) != nil {
		t.Fatal("valid owned ACE rejected")
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"empty":              func(b []byte) []byte { return nil },
		"short ACL":          func(b []byte) []byte { return b[:7] },
		"revision":           func(b []byte) []byte { b[0] = 0; return b },
		"length":             func(b []byte) []byte { b[2]--; return b },
		"no ACE":             func(b []byte) []byte { b[4] = 0; return b },
		"too many ACEs":      func(b []byte) []byte { b[4] = 17; return b },
		"missing next ACE":   func(b []byte) []byte { b[4] = 2; return b },
		"short ACE":          func(b []byte) []byte { b[10] = 12; return b },
		"unaligned ACE":      func(b []byte) []byte { b[10] = 17; return b },
		"ACE overrun":        func(b []byte) []byte { b[10] = 252; return b },
		"deny ACE":           func(b []byte) []byte { b[8] = 1; return b },
		"object ACE":         func(b []byte) []byte { b[8] = 5; return b },
		"SID revision":       func(b []byte) []byte { b[16] = 0; return b },
		"SID count overrun":  func(b []byte) []byte { b[17] = 15; return b },
		"SID count invalid":  func(b []byte) []byte { b[17] = 16; return b },
		"SID trailing bytes": func(b []byte) []byte { b[17]--; return b },
		"untrusted SID":      func(b []byte) []byte { b[len(b)-1] = 1; return b },
	} {
		t.Run(name, func(t *testing.T) {
			if privateACLBytes(mutate(bytes.Clone(valid)), owner) != ErrUnsafe {
				t.Fatal("malformed ACL accepted")
			}
		})
	}
	if privateACLBytes(valid, nil) != ErrUnsafe {
		t.Fatal("missing owner accepted")
	}
}

func grantEveryone(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnsafeWindowsDACLRejectedWithoutRepair(t *testing.T) {
	dir := privateDirectory(t)
	grantEveryone(t, dir)
	before, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := Open(dir, true); err != ErrUnsafe {
		if s != nil {
			s.Close()
		}
		t.Fatal("Everyone directory grant accepted")
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if sd.String() != before.String() {
		t.Fatal("unsafe existing ACL was modified")
	}
	private := privateDirectory(t)
	r := openStore(t, private, false)
	file := filepath.Join(private, latestName)
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	grantEveryone(t, file)
	if data, err := r.Read(serverID); data != nil || err != ErrUnsafe {
		t.Fatal("Everyone snapshot grant accepted")
	}
}

func TestNullWindowsDACLRejected(t *testing.T) {
	dir := privateDirectory(t)
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(dir, false); err != ErrUnsafe {
		if s != nil {
			s.Close()
		}
		t.Fatal("null directory DACL accepted")
	}
}
