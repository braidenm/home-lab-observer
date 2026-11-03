//go:build linux

package actionprofile

import (
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func noACL(fd int, dir bool) bool {
	if _, err := unix.Fgetxattr(fd, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		return false
	}
	if dir {
		if _, err := unix.Fgetxattr(fd, "system.posix_acl_default", nil); !errors.Is(err, unix.ENODATA) {
			return false
		}
	}
	return true
}
func stable(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
func rootDirectory(fd int, exactMode uint32) bool {
	var s unix.Stat_t
	return unix.Fstat(fd, &s) == nil && s.Mode&unix.S_IFMT == unix.S_IFDIR && s.Uid == 0 && s.Gid == 0 &&
		s.Mode&007022 == 0 && (exactMode == 0 || s.Mode == unix.S_IFDIR|exactMode) && noACL(fd, true)
}
func namedSame(parent int, name string, fd int) bool {
	var path, opened unix.Stat_t
	return unix.Fstatat(parent, name, &path, unix.AT_SYMLINK_NOFOLLOW) == nil && unix.Fstat(fd, &opened) == nil && stable(path, opened)
}
func namespaceFD(fd int) (unix.Stat_t, error) {
	var s unix.Stat_t
	var fs unix.Statfs_t
	kind, err := unix.IoctlRetInt(fd, unix.NS_GET_NSTYPE)
	if err != nil || kind != unix.CLONE_NEWUSER || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.NSFS_MAGIC || unix.Fstat(fd, &s) != nil {
		return s, ErrProfile
	}
	return s, nil
}

// VerifyHostNamespace depends on a separately root-provisioned actual host NSFS
// reference. It never mounts/adopts/repairs one or substitutes UID-map spelling.
func VerifyHostNamespace() (result error) {
	result = ErrProfile
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return
	}
	defer func() {
		if unix.Close(root) != nil {
			result = ErrProfile
		}
	}()
	run, err := unix.Openat(root, "run", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return
	}
	defer func() {
		if unix.Close(run) != nil {
			result = ErrProfile
		}
	}()
	base, err := unix.Openat(run, "home-lab-observer-actions", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return
	}
	defer func() {
		if unix.Close(base) != nil {
			result = ErrProfile
		}
	}()
	if !rootDirectory(root, 0755) || !rootDirectory(run, 0) || !rootDirectory(base, 0755) || !namedSame(root, "run", run) || !namedSame(run, "home-lab-observer-actions", base) {
		return
	}
	reference, err := unix.Openat(base, "host-userns", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer func() {
		if unix.Close(reference) != nil {
			result = ErrProfile
		}
	}()
	// Intentional fixed kernel proc namespace magic link, NOT a generic symlink exception.
	self, err := unix.Open("/proc/self/ns/user", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return
	}
	defer func() {
		if unix.Close(self) != nil {
			result = ErrProfile
		}
	}()
	a, e1 := namespaceFD(reference)
	b, e2 := namespaceFD(self)
	if e1 != nil || e2 != nil || a.Dev != b.Dev || a.Ino != b.Ino || !namedSame(base, "host-userns", reference) ||
		!namedSame(root, "run", run) || !namedSame(run, "home-lab-observer-actions", base) || !rootDirectory(root, 0755) || !rootDirectory(run, 0) || !rootDirectory(base, 0755) {
		return
	}
	return nil
}

func readAccountFile(name string) (_ string, result error) {
	if name != "passwd" && name != "group" && name != "nsswitch.conf" {
		return "", ErrProfile
	}
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", ErrProfile
	}
	defer func() {
		if unix.Close(root) != nil {
			result = ErrProfile
		}
	}()
	etc, err := unix.Openat(root, "etc", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", ErrProfile
	}
	defer func() {
		if unix.Close(etc) != nil {
			result = ErrProfile
		}
	}()
	if !rootDirectory(root, 0755) || !rootDirectory(etc, 0) || !namedSame(root, "etc", etc) {
		return "", ErrProfile
	}
	fd, err := unix.Openat(etc, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return "", ErrProfile
	}
	f := os.NewFile(uintptr(fd), "action-profile-local-account")
	defer func() {
		if f.Close() != nil {
			result = ErrProfile
		}
	}()
	var before, after unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode != unix.S_IFREG|0644 || before.Uid != 0 || before.Gid != 0 || before.Nlink != 1 || before.Size < 1 || before.Size > 256*1024 || !noACL(fd, false) || !namedSame(etc, name, fd) {
		return "", ErrProfile
	}
	raw, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil || int64(len(raw)) != before.Size || unix.Fstat(fd, &after) != nil || !stable(before, after) || !namedSame(etc, name, fd) || !namedSame(root, "etc", etc) || !rootDirectory(etc, 0) {
		return "", ErrProfile
	}
	return string(raw), nil
}
func loadIdentities() (localRoles, error) {
	p, e1 := readAccountFile("passwd")
	g, e2 := readAccountFile("group")
	n, e3 := readAccountFile("nsswitch.conf")
	if e1 != nil || e2 != nil || e3 != nil {
		return localRoles{}, ErrProfile
	}
	return resolveIdentities(p, g, n)
}
func processGroups(uid, gid uint32, allowed ...uint32) bool {
	if uint32(os.Getuid()) != uid || uint32(os.Geteuid()) != uid || uint32(os.Getgid()) != gid || uint32(os.Getegid()) != gid {
		return false
	}
	actual, err := os.Getgroups()
	if err != nil {
		return false
	}
	set := map[uint32]bool{gid: true}
	want := map[uint32]bool{}
	for _, g := range actual {
		if g < 0 {
			return false
		}
		set[uint32(g)] = true
	}
	for _, g := range allowed {
		if g == 0 {
			return false
		}
		want[g] = true
	}
	if len(set) != len(want) {
		return false
	}
	for g := range set {
		if !want[g] {
			return false
		}
	}
	return true
}
func VerifyWorker() (Identity, error) {
	if VerifyHostNamespace() != nil {
		return Identity{}, ErrProfile
	}
	r, err := loadIdentities()
	i := r.identity
	if err != nil || !processGroups(i.WorkerUID, i.WorkerGID, i.WorkerGID, i.IPCGID) {
		return Identity{}, ErrProfile
	}
	return i, nil
}
func VerifyBroker(approvedEngineGID uint32) (Identity, error) {
	if VerifyHostNamespace() != nil {
		return Identity{}, ErrProfile
	}
	r, err := loadIdentities()
	i := r.identity
	if err != nil || approvedEngineGID == 0 || approvedEngineGID == i.WorkerGID || approvedEngineGID == i.BrokerGID || approvedEngineGID == i.IPCGID ||
		!processGroups(i.BrokerUID, i.BrokerGID, i.BrokerGID, i.IPCGID, approvedEngineGID) {
		return Identity{}, ErrProfile
	}
	for _, g := range r.brokerMemberships {
		if g != i.BrokerGID && g != i.IPCGID && g != approvedEngineGID {
			return Identity{}, ErrProfile
		}
	}
	return i, nil
}
func VerifyAdmin() (Identity, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 || VerifyHostNamespace() != nil {
		return Identity{}, ErrProfile
	}
	r, err := loadIdentities()
	return r.identity, err
}
func peer(raw syscall.RawConn, uid, gid uint32) error {
	if raw == nil {
		return ErrProfile
	}
	var result error = ErrProfile
	if raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err == nil && cred.Pid > 0 && cred.Uid == uid && cred.Gid == gid {
			result = nil
		}
	}) != nil {
		return ErrProfile
	}
	return result
}
func VerifyWorkerPeer(raw syscall.RawConn, expected Identity) error {
	// Called in the already validated broker. Re-read protected role identities;
	// caller-supplied numbers cannot replace the fixed account namespace.
	if VerifyHostNamespace() != nil {
		return ErrProfile
	}
	r, err := loadIdentities()
	if err != nil || r.identity != expected || uint32(os.Geteuid()) != expected.BrokerUID {
		return ErrProfile
	}
	return peer(raw, expected.WorkerUID, expected.WorkerGID)
}
func VerifyBrokerPeer(raw syscall.RawConn, expected Identity) error {
	if VerifyHostNamespace() != nil {
		return ErrProfile
	}
	r, err := loadIdentities()
	if err != nil || r.identity != expected || uint32(os.Geteuid()) != expected.WorkerUID && os.Geteuid() != 0 {
		return ErrProfile
	}
	return peer(raw, expected.BrokerUID, expected.BrokerGID)
}
func VerifyAdminPeer(raw syscall.RawConn, expected Identity) error {
	if VerifyHostNamespace() != nil {
		return ErrProfile
	}
	r, err := loadIdentities()
	if err != nil || r.identity != expected || uint32(os.Geteuid()) != expected.BrokerUID {
		return ErrProfile
	}
	// Kernel credentials are interpreted in the verified initial host namespace.
	if raw == nil {
		return ErrProfile
	}
	var result error = ErrProfile
	if raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e == nil && cred.Pid > 0 && cred.Uid == 0 {
			result = nil
		}
	}) != nil {
		return ErrProfile
	}
	return result
}
