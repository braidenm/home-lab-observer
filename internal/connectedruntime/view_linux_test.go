//go:build linux

package connectedruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUploaderViewUsesOnlyExactInertScaffold(t *testing.T) {
	for _, change := range []string{"valid", "home", "home-error", "link", "file", "mode", "uid", "gid", "special", "acl", "child", "procfs", "sysfs", "read-error", "root-readable", "root-searchable"} {
		t.Run(change, func(t *testing.T) {
			read := func(path string) (viewDirectory, error) {
				var spec scaffoldDirectory
				for _, candidate := range uploaderScaffold[:5] {
					if path == "/"+candidate.name {
						spec = candidate
					}
				}
				if spec.name == "" {
					t.Fatal("unexpected path")
				}
				d := viewDirectory{stat: unix.Stat_t{Mode: unix.S_IFDIR | spec.mode}, filesystem: unix.EXT4_SUPER_MAGIC, noACL: true, denied: spec.name == "root"}
				if spec.name == "proc" {
					switch change {
					case "link":
						d.stat.Mode = unix.S_IFLNK | 0555
					case "file":
						d.stat.Mode = unix.S_IFREG | 0555
					case "mode":
						d.stat.Mode = unix.S_IFDIR | 0755
					case "uid":
						d.stat.Uid = 17
					case "gid":
						d.stat.Gid = 17
					case "special":
						d.stat.Mode |= unix.S_ISGID
					case "acl":
						d.noACL = false
					case "child":
						d.entries = []string{"not-emitted"}
					case "procfs":
						d.filesystem = unix.PROC_SUPER_MAGIC
					case "sysfs":
						d.filesystem = unix.SYSFS_MAGIC
					case "read-error":
						return d, unix.EIO
					}
				}
				return d, nil
			}
			access := func(_ string, mask uint32) error {
				if (change == "root-readable" && mask == unix.R_OK) || (change == "root-searchable" && mask == unix.X_OK) {
					return nil
				}
				return unix.EACCES
			}
			lstat := func(string) (os.FileInfo, error) {
				if change == "home" {
					return nil, nil
				}
				if change == "home-error" {
					return nil, unix.EIO
				}
				return nil, os.ErrNotExist
			}
			if (checkUploaderView(read, access, lstat) == nil) != (change == "valid") {
				t.Fatal("view boundary mismatch")
			}
		})
	}
}

func TestPhysicalScaffoldRejectsEveryUnexpectedLeafAndMetadataDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range uploaderRootChildren {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range uploaderScaffold {
		if err := os.MkdirAll(filepath.Join(root, spec.name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, spec := range uploaderScaffold {
			_ = os.Chmod(filepath.Join(root, spec.name), 0755)
		}
	})
	for _, spec := range uploaderScaffold {
		if err := os.Chmod(filepath.Join(root, spec.name), os.FileMode(spec.mode)); err != nil {
			t.Fatal(err)
		}
	}
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	if checkPhysicalScaffold(root, uid, gid) != nil {
		t.Fatal("accepted physical scaffold refused")
	}
	foreign := filepath.Join(root, "foreign-top-level")
	if err := os.Mkdir(foreign, 0755); err != nil {
		t.Fatal(err)
	}
	if checkPhysicalScaffold(root, uid, gid) == nil {
		t.Fatal("extra root member admitted")
	}
	if err := os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	for _, spec := range uploaderScaffold {
		path := filepath.Join(root, spec.name)
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
		extra := filepath.Join(path, "foreign-value-not-emitted")
		if err := os.WriteFile(extra, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, os.FileMode(spec.mode)); err != nil {
			t.Fatal(err)
		}
		if checkPhysicalScaffold(root, uid, gid) == nil {
			t.Fatalf("extra member accepted in %s", spec.name)
		}
		_ = os.Chmod(path, 0755)
		if err := os.Remove(extra); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(path, os.FileMode(spec.mode))
	}
	if err := os.Chmod(filepath.Join(root, "usr"), 0775); err != nil {
		t.Fatal(err)
	}
	if checkPhysicalScaffold(root, uid, gid) == nil {
		t.Fatal("writable physical scaffold admitted")
	}
	_ = os.Chmod(filepath.Join(root, "usr"), 0755)
	if err := os.Rename(filepath.Join(root, "usr"), filepath.Join(root, "old-usr")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("old-usr", filepath.Join(root, "usr")); err != nil {
		t.Fatal(err)
	}
	if checkPhysicalScaffold(root, uid, gid) == nil {
		t.Fatal("scaffold link admitted")
	}
}

func TestPinnedNamespaceIdentityRejectsSubstitutionAndMissingSources(t *testing.T) {
	root := "/fixed/root"
	sources := map[string]string{}
	for _, name := range []string{"/bin/observer-connected-uploader", "/etc/home-lab-observer-connected/installed.json", "/handoff", "/activation", "/state/ledger", "/state/status"} {
		sources[name] = root + name
	}
	objects := map[string]unix.Stat_t{".": {Dev: 2, Ino: 1, Mode: unix.S_IFDIR | 0755}}
	for _, spec := range uploaderScaffold[:6] {
		objects[spec.name] = unix.Stat_t{Dev: 2, Ino: uint64(len(objects) + 1), Mode: unix.S_IFDIR | spec.mode}
	}
	for target := range sources {
		objects[strings.TrimPrefix(target, "/")] = unix.Stat_t{Dev: 2, Ino: uint64(len(objects) + 1), Mode: unix.S_IFDIR | 0755}
	}
	physical := func(path string) (unix.Stat_t, error) {
		if path == root {
			return objects["."], nil
		}
		s, ok := objects[strings.TrimPrefix(path, root+"/")]
		if !ok {
			return s, unix.ENOENT
		}
		return s, nil
	}
	for changed := range objects {
		actual := func(name string) (unix.Stat_t, error) {
			s := objects[name]
			if name == changed {
				s.Ino++
			}
			return s, nil
		}
		if checkNamespaceBindings(root, sources, actual, physical) == nil {
			t.Fatal("substituted namespace object admitted")
		}
	}
	actual := func(name string) (unix.Stat_t, error) {
		s, ok := objects[name]
		if !ok {
			return s, unix.ENOENT
		}
		return s, nil
	}
	if checkNamespaceBindings(root, sources, actual, physical) != nil {
		t.Fatal("identical objects refused")
	}
	delete(sources, "/handoff")
	if checkNamespaceBindings(root, sources, actual, physical) == nil {
		t.Fatal("missing binding admitted")
	}
}

func TestNamespaceTraversalNeverFollowsInnerLinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(root, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	for _, name := range []string{"link", "link/child", "../real", "", "absent"} {
		if _, err := viewObjectAt(fd, name); err == nil {
			t.Fatal("unsafe namespace path admitted")
		}
	}
	if _, err := viewObjectAt(fd, "real"); err != nil {
		t.Fatal("real directory refused")
	}
}

func TestViewDirectoryRefusesActualACLAndNonDirectory(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readViewDirectory(file); err == nil {
		t.Fatal("regular file accepted")
	}
	if err := os.Symlink("file", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := readViewDirectory(filepath.Join(root, "link")); err == nil {
		t.Fatal("link accepted")
	}
	acl := []byte{2, 0, 0, 0, 1, 0, 7, 0, 255, 255, 255, 255, 2, 0, 4, 0, 254, 255, 0, 0, 4, 0, 5, 0, 255, 255, 255, 255, 16, 0, 5, 0, 255, 255, 255, 255, 32, 0, 5, 0, 255, 255, 255, 255}
	if err := unix.Setxattr(root, "system.posix_acl_access", acl, 0); err != nil {
		if err == unix.ENOTSUP {
			t.Skip("fixture filesystem lacks POSIX ACLs")
		}
		t.Fatal(err)
	}
	if _, err := readViewDirectory(root); err == nil {
		t.Fatal("ACL accepted")
	}
}

func TestOnlyExactEmptyPrivateMqueueIsAccepted(t *testing.T) {
	for _, change := range []string{"valid", "mode", "owner", "group", "acl", "filesystem", "entry", "denied"} {
		d := viewDirectory{stat: unix.Stat_t{Mode: unix.S_IFDIR | 01777}, filesystem: mqueueMagic, noACL: true}
		switch change {
		case "mode":
			d.stat.Mode = unix.S_IFDIR | 0755
		case "owner":
			d.stat.Uid = 10
		case "group":
			d.stat.Gid = 10
		case "acl":
			d.noACL = false
		case "filesystem":
			d.filesystem = unix.EXT4_SUPER_MAGIC
		case "entry":
			d.entries = []string{"private-not-emitted"}
		case "denied":
			d.denied = true
		}
		if safePrivateMqueue(d) != (change == "valid") {
			t.Fatal("private queue policy mismatch")
		}
	}
}
