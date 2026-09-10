package logprocess

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
	"golang.org/x/sys/windows"
)

const syntheticOwner = "S-1-5-21-100-200-300-1001"

func TestWindowsACLReplacementPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, dacl      string
		directory, safe bool
	}{
		{"owner-only", "(A;;FA;;;" + syntheticOwner + ")", false, true},
		{"owner-rights", "(A;;FA;;;OW)", false, true},
		{"unknown-mask", "(A;;0x02000000;;;WD)", false, false},
		{"public-read", "(A;;FRFX;;;WD)", false, true},
		{"public-write", "(A;;FW;;;WD)", false, false},
		{"public-delete", "(A;;SD;;;WD)", false, false},
		{"public-takeover", "(A;;WDWO;;;WD)", true, false},
		{"root-create-only", "(A;;0x00000006;;;BU)", true, true},
		{"same-bits-file-write", "(A;;0x00000006;;;BU)", false, false},
		{"directory-delete-child", "(A;;0x00000040;;;BU)", true, false},
		{"directory-attributes", "(A;;0x00000100;;;BU)", true, false},
		{"inherit-only", "(A;OICIIO;GA;;;WD)", true, true},
		{"effective-inherited-write", "(A;ID;GA;;;WD)", false, false},
		{"deny-does-not-rescue-allow", "(D;;FW;;;WD)(A;;FW;;;WD)", false, false},
		{"administrator", "(A;;FA;;;BA)", false, true},
		{"system", "(A;;FA;;;SY)", false, true},
		{"trusted-installer", "(A;;FA;;;" + trustedInstallerSID + ")", true, true},
		{"other-service", "(A;;FA;;;S-1-5-80-1-2-3-4-5)", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString("O:" + syntheticOwner + "D:" + tc.dacl)
			if err != nil {
				t.Fatal(err)
			}
			if got := safeWindowsDescriptor(sd, tc.directory, syntheticOwner); got != tc.safe {
				t.Fatalf("safe=%v want=%v", got, tc.safe)
			}
		})
	}
	for _, sddl := range []string{"O:WDD:(A;;FR;;;WD)", "O:" + syntheticOwner + "D:NO_ACCESS_CONTROL"} {
		sd, err := windows.SecurityDescriptorFromString(sddl)
		if err != nil {
			t.Fatal(err)
		}
		if safeWindowsDescriptor(sd, false, syntheticOwner) {
			t.Fatal("untrusted owner/null DACL accepted")
		}
	}
	large, err := windows.SecurityDescriptorFromString("O:" + syntheticOwner + "D:" + strings.Repeat("(A;;FR;;;WD)", maxHelperACEs+1))
	if err != nil {
		t.Fatal(err)
	}
	if safeWindowsDescriptor(large, false, syntheticOwner) {
		t.Fatal("excessive ACL accepted")
	}
}

func TestWindowsPathSelectionRejectsAlternateAuthorities(t *testing.T) {
	for _, path := range []string{`relative.exe`, `C:relative.exe`, `\\server\share\observer.exe`, `\\?\C:\observer.exe`, `\\.\C:\observer.exe`, `C:\observer.exe:stream`, `C:\safe\..\observer.exe`} {
		if validWindowsExecutablePath(path) {
			t.Fatalf("unsafe path accepted %q", path)
		}
	}
	if !validWindowsExecutablePath(`C:\safe\observer.exe`) {
		t.Fatal("ordinary local path refused")
	}
}

func TestWindowsTemporaryFileIdentityAndMinimalEnvironment(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic-observer.exe")
	if err := os.WriteFile(file, []byte("synthetic-not-executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ownerfs.RestrictDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := ownerfs.RestrictFile(file); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyWindowsNode(file, false, user.User.Sid.String()); err != nil {
		t.Fatal("trusted file node refused")
	}
	if _, err := verifyWindowsNode(dir, true, user.User.Sid.String()); err != nil {
		t.Fatal("trusted directory node refused")
	}
	t.Setenv("SystemRoot", `C:\synthetic-poison`)
	env, actual, err := windowsHelperEnvironment()
	if err != nil || len(env) != 4 || env[3] != "SystemRoot="+actual || strings.Contains(strings.Join(env, ";"), "synthetic-poison") {
		t.Fatal("inherited SystemRoot")
	}
	link := filepath.Join(dir, "hardlink.exe")
	if err := os.Link(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyWindowsNode(file, false, user.User.Sid.String()); !errors.Is(err, ErrHelperUnavailable) {
		t.Fatal("hardlinked executable accepted")
	}
}

func TestWindowsCompleteTrustedPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic-observer.exe")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ownerfs.RestrictDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := ownerfs.RestrictFile(file); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for path := filepath.Dir(dir); ; path = filepath.Dir(path) {
		if _, err := verifyWindowsNode(path, true, user.User.Sid.String()); err != nil {
			if os.Getenv("GITHUB_ACTIONS") == "true" {
				t.Fatal("CI must provide a trusted standard install-path fixture")
			}
			t.Skip("preexisting test-host ancestors grant replacement to other principals; policy intentionally refuses")
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	spec, err := resolvePlatformHelper(file, "")
	if err != nil || spec.path != file || len(spec.args) != 1 || spec.args[0] != "__log-helper" {
		t.Fatalf("fixed same-binary identity: %v", err)
	}
}

func TestWindowsRejectsUnsafeTemporaryExecutableACL(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic-observer.exe")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FW;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := resolvePlatformHelper(file, ""); !errors.Is(err, ErrHelperUnavailable) {
		t.Fatal("unsafe executable ACL accepted")
	}
}

func TestWindowsFileInformationBounds(t *testing.T) {
	valid := windows.ByHandleFileInformation{NumberOfLinks: 1, FileSizeLow: 1}
	if !safeWindowsFileInformation(valid, false) {
		t.Fatal("ordinary file refused")
	}
	for _, bad := range []windows.ByHandleFileInformation{
		{NumberOfLinks: 1, FileSizeLow: 1, FileAttributes: windows.FILE_ATTRIBUTE_REPARSE_POINT},
		{NumberOfLinks: 1, FileSizeLow: 1, FileAttributes: windows.FILE_ATTRIBUTE_DIRECTORY},
		{NumberOfLinks: 2, FileSizeLow: 1},
		{NumberOfLinks: 1, FileSizeLow: 0},
		{NumberOfLinks: 1, FileSizeLow: maxHelperBytes + 1},
		{NumberOfLinks: 1, FileSizeHigh: 1},
	} {
		if safeWindowsFileInformation(bad, false) {
			t.Fatal("unsafe native file information accepted")
		}
	}
}

func TestWindowsReparseLinkRefused(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic.exe")
	link := filepath.Join(dir, "linked.exe")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, link); err != nil {
		t.Skip("test account cannot create symbolic-link fixture")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyWindowsNode(link, false, user.User.Sid.String()); !errors.Is(err, ErrHelperUnavailable) {
		t.Fatal("reparse link accepted")
	}
}
