//go:build linux

package connectedruntime

import (
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

type viewDirectory struct {
	stat          unix.Stat_t
	filesystem    int64
	entries       []string
	noACL, denied bool
}

type scaffoldDirectory struct {
	name     string
	mode     uint32
	children []string
}

// Linux 6.8 ipc/mqueue.c; this ABI constant is not exported by x/sys/unix.
const mqueueMagic = 0x19800202

// Exact empty mount points observed on the supported systemd 255 profile.
var uploaderScaffold = []scaffoldDirectory{
	{"root", 0750, nil}, {"usr", 0755, nil}, {"var", 0755, nil},
	{"proc", 0555, nil}, {"sys", 0555, nil},
	{"dev", 0555, []string{"mqueue"}}, {"dev/mqueue", 0755, nil},
	{"run", 0555, []string{"systemd"}}, {"run/systemd", 0755, []string{"incoming"}},
	{"run/systemd/incoming", 0755, nil},
}

var uploaderRootChildren = []string{"activation", "bin", "dev", "etc", "handoff", "proc", "root", "run", "state", "sys", "usr", "var"}

func safeScaffold(d viewDirectory, spec scaffoldDirectory, uid, gid uint32, inaccessible bool) bool {
	if d.stat.Mode != unix.S_IFDIR|spec.mode || d.stat.Uid != uid || d.stat.Gid != gid || !d.noACL ||
		d.filesystem == unix.PROC_SUPER_MAGIC || d.filesystem == unix.SYSFS_MAGIC {
		return false
	}
	if inaccessible {
		return d.denied && len(d.entries) == 0
	}
	return !d.denied && slices.Equal(d.entries, spec.children)
}

func checkUploaderView(read func(string) (viewDirectory, error), access func(string, uint32) error, lstat func(string) (os.FileInfo, error)) error {
	if _, err := lstat("/home"); !os.IsNotExist(err) {
		return ErrUnsafe
	}
	for _, spec := range uploaderScaffold[:5] {
		path := "/" + spec.name
		d, err := read(path)
		if err != nil || !safeScaffold(d, spec, 0, 0, spec.name == "root") {
			return ErrUnsafe
		}
		if spec.name == "root" && (access(path, unix.R_OK) != unix.EACCES || access(path, unix.X_OK) != unix.EACCES) {
			return ErrUnsafe
		}
	}
	return nil
}

// CheckInstalledUploaderScaffold is a root-only fixed-path audit. In particular,
// root proves /root empty before the worker's same-process denial check.
func CheckInstalledUploaderScaffold() error {
	if os.Geteuid() != 0 || os.Getuid() != 0 {
		return ErrUnsafe
	}
	return checkPhysicalScaffold("/var/lib/home-lab-observer-connected/uploader-root", 0, 0)
}

func CheckInstalledRichProcessesUploaderScaffold() error {
	if os.Geteuid() != 0 || os.Getuid() != 0 {
		return ErrUnsafe
	}
	return checkPhysicalScaffold("/var/lib/home-lab-observer-connected-rich/uploader-root", 0, 0)
}

func checkPhysicalScaffold(root string, uid, gid uint32) error {
	d, err := readViewDirectoryLimit(root, len(uploaderRootChildren)+1)
	if err != nil || !safeScaffold(d, scaffoldDirectory{"", 0755, uploaderRootChildren}, uid, gid, false) {
		return ErrUnsafe
	}
	for _, spec := range uploaderScaffold {
		d, err := readViewDirectory(root + "/" + spec.name)
		// LoadCredential creates this inert parent in RootDirectory. The live
		// per-unit credential mount is separate; never accept credential bytes,
		// aliases, writable metadata or unknown children in the physical tree.
		if spec.name == "run" && slices.Equal(d.entries, []string{"credentials", "systemd"}) {
			credentials, readErr := readViewDirectory(root + "/run/credentials")
			if readErr != nil || !safeScaffold(credentials, scaffoldDirectory{"run/credentials", 0755, nil}, uid, gid, false) {
				return ErrUnsafe
			}
			spec.children = []string{"credentials", "systemd"}
		}
		if err != nil || !safeScaffold(d, spec, uid, gid, false) {
			return ErrUnsafe
		}
	}
	return nil
}

func readViewDirectory(path string) (viewDirectory, error) {
	return readViewDirectoryLimit(path, 3)
}

func readViewDirectoryLimit(path string, limit int) (viewDirectory, error) {
	var d viewDirectory
	if unix.Lstat(path, &d.stat) != nil || d.stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return d, ErrUnsafe
	}
	d.noACL = true
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		_, err := unix.Lgetxattr(path, name, nil)
		if err != unix.ENODATA && err != unix.ENOTSUP {
			d.noACL = false
			return d, ErrUnsafe
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.EACCES {
		d.denied = true
		return d, nil
	}
	if err != nil {
		return d, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "view-directory")
	defer f.Close()
	var opened unix.Stat_t
	var filesystem unix.Statfs_t
	if unix.Fstat(fd, &opened) != nil || !sameViewObject(d.stat, opened) || opened.Mode != d.stat.Mode ||
		opened.Uid != d.stat.Uid || opened.Gid != d.stat.Gid || unix.Fstatfs(fd, &filesystem) != nil {
		return d, ErrUnsafe
	}
	d.filesystem = filesystem.Type
	d.entries, err = f.Readdirnames(limit)
	if err != nil && err != io.EOF {
		return d, ErrUnsafe
	}
	slices.Sort(d.entries)
	return d, nil
}

func sameViewObject(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode&unix.S_IFMT == b.Mode&unix.S_IFMT
}

// CheckUploaderNamespace compares only fixed paths and object identities. A
// read-only remount may have another mount ID, but cannot substitute host data.
// The caller audits PID/start identity before and after this bounded check.
func CheckUploaderNamespace(pid int, root string, sources map[string]string) error {
	return checkUploaderNamespace(pid, root, sources, false)
}

func CheckRichProcessesUploaderNamespace(pid int, root string, sources map[string]string) error {
	return checkUploaderNamespace(pid, root, sources, true)
}

func checkUploaderNamespace(pid int, root string, sources map[string]string, rich bool) error {
	expectedRoot := "/var/lib/home-lab-observer-connected/uploader-root"
	if rich {
		expectedRoot = "/var/lib/home-lab-observer-connected-rich/uploader-root"
	}
	if os.Geteuid() != 0 || pid < 2 || root != expectedRoot {
		return ErrUnsafe
	}
	proc := "/proc/" + strconv.Itoa(pid) + "/root"
	// Follow this one procfs magic link intentionally, then pin the namespace
	// root. All subsequent worker paths are traversed relative to this handle.
	fd, err := unix.Open(proc, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	defer unix.Close(fd)
	if checkNamespaceBindingsForProfile(root, sources, rich, func(name string) (unix.Stat_t, error) { return viewObjectAt(fd, name) }, func(path string) (unix.Stat_t, error) {
		var s unix.Stat_t
		err := unix.Lstat(path, &s)
		return s, err
	}) != nil {
		return ErrUnsafe
	}
	// PrivateIPC may mount a distinct empty mqueue filesystem. With API mounts
	// disabled, an unchanged inert physical placeholder is safe as well. Neither
	// branch accepts a host mount or populated IPC directory.
	queue, err := readViewDirectory("/proc/self/fd/" + strconv.Itoa(fd) + "/dev/mqueue")
	if err != nil {
		return ErrUnsafe
	}
	var physical unix.Stat_t
	if unix.Lstat(root+"/dev/mqueue", &physical) != nil {
		return ErrUnsafe
	}
	var workerIPC, hostIPC unix.Stat_t
	if unix.Stat("/proc/"+strconv.Itoa(pid)+"/ns/ipc", &workerIPC) != nil ||
		unix.Stat("/proc/self/ns/ipc", &hostIPC) != nil || sameViewObject(workerIPC, hostIPC) {
		return ErrUnsafe
	}
	if sameViewObject(queue.stat, physical) && safeScaffold(queue, uploaderScaffold[6], 0, 0, false) {
		return nil
	}
	if !safePrivateMqueue(queue) {
		return ErrUnsafe
	}
	return nil
}

func safePrivateMqueue(d viewDirectory) bool {
	return d.stat.Mode == unix.S_IFDIR|01777 && d.stat.Uid == 0 && d.stat.Gid == 0 && d.noACL &&
		!d.denied && d.filesystem == mqueueMagic && len(d.entries) == 0
}

func checkNamespaceBindings(root string, sources map[string]string, actual, physical func(string) (unix.Stat_t, error)) error {
	return checkNamespaceBindingsForProfile(root, sources, false, actual, physical)
}

func checkNamespaceBindingsForProfile(root string, sources map[string]string, rich bool, actual, physical func(string) (unix.Stat_t, error)) error {
	targets := []string{"/bin/observer-connected-uploader", "/etc/home-lab-observer-connected/installed.json", "/handoff", "/activation", "/state/ledger", "/state/status"}
	if rich {
		targets = []string{"/bin/observer-connected-rich-uploader", "/etc/home-lab-observer-connected-rich/installed.json", "/etc/home-lab-observer-connected-rich/collection-policy.json", "/handoff", "/activation", "/state/ledger", "/state/status"}
	}
	if len(sources) != len(targets) {
		return ErrUnsafe
	}
	match := func(name, source string) bool {
		a, e := actual(name)
		b, f := physical(source)
		return e == nil && f == nil && sameViewObject(a, b)
	}
	if !match(".", root) {
		return ErrUnsafe
	}
	for _, spec := range uploaderScaffold[:6] {
		if !match(spec.name, root+"/"+spec.name) {
			return ErrUnsafe
		}
	}
	for _, target := range targets {
		source, ok := sources[target]
		if !ok || !match(strings.TrimPrefix(target, "/"), source) {
			return ErrUnsafe
		}
	}
	return nil
}

func viewObjectAt(root int, name string) (unix.Stat_t, error) {
	var s unix.Stat_t
	fd, err := unix.Dup(root)
	if err != nil {
		return s, ErrUnsafe
	}
	defer func() { unix.Close(fd) }()
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == ".." {
			return s, ErrUnsafe
		}
		next, err := unix.Openat(fd, part, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return s, ErrUnsafe
		}
		unix.Close(fd)
		fd = next
		if unix.Fstat(fd, &s) != nil || s.Mode&unix.S_IFMT == unix.S_IFLNK {
			return s, ErrUnsafe
		}
	}
	return s, nil
}
