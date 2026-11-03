//go:build linux

package actionprofile

import (
	"net"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNamespaceHandleRequiresActualNSFSAndUserNamespaceType(t *testing.T) {
	// Ordinary read-only kernel metadata. This does NOT provision a host reference
	// or certify an installed profile/initial-host namespace.
	user, err := os.Open("/proc/self/ns/user")
	if err != nil {
		t.Fatal("owned user namespace handle")
	}
	defer user.Close()
	stat, err := namespaceFD(int(user.Fd()))
	if err != nil || stat.Ino == 0 {
		t.Fatal("NSFS user handle rejected")
	}
	mount, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal("mount namespace handle")
	}
	defer mount.Close()
	if _, err := namespaceFD(int(mount.Fd())); err == nil {
		t.Fatal("foreign namespace type accepted")
	}
	marker, err := os.CreateTemp(t.TempDir(), "not-a-namespace")
	if err != nil {
		t.Fatal("owned regular file")
	}
	defer marker.Close()
	if _, err := namespaceFD(int(marker.Fd())); err == nil {
		t.Fatal("regular marker became namespace provenance")
	}
}

func TestPeerPrimitiveReadsActualKernelCredentialsNotPIDOnly(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("owned peer pair")
	}
	defer unix.Close(fds[1])
	file := os.NewFile(uintptr(fds[0]), "owned-peer")
	conn, err := net.FileConn(file)
	file.Close()
	if err != nil {
		t.Fatal("owned connection")
	}
	defer conn.Close()
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err != nil {
		t.Fatal("owned raw socket")
	}
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	if peer(raw, uid, gid) != nil || peer(raw, uid+1, gid) == nil || peer(raw, uid, gid+1) == nil || peer(nil, uid, gid) == nil {
		t.Fatal("kernel UID/GID correlation bypassed")
	}
	// Matching our own synthetic expectations is not service-role admission.
	// Public entry points additionally resolve protected static roles and host NSFS.
}

func TestWitnessStabilityIgnoresOnlyReadAtime(t *testing.T) {
	before := unix.Stat_t{Dev: 1, Ino: 2, Mode: unix.S_IFREG | 0644, Uid: 0, Gid: 0, Nlink: 1, Size: 17, Mtim: unix.Timespec{Sec: 3}, Ctim: unix.Timespec{Sec: 4}}
	after := before
	after.Atim.Sec++
	if !stable(before, after) {
		t.Fatal("cold relatime read falsely changed identity")
	}
	after.Ino++
	if stable(before, after) {
		t.Fatal("replaced object retained witness")
	}
}
