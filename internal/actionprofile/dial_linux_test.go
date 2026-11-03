//go:build linux

package actionprofile

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func ownedEndpoint(t *testing.T) (*endpointPin, string, *net.UnixListener) {
	return ownedEndpointRole(t, false)
}

func ownedEndpointRole(t *testing.T, admin bool) (*endpointPin, string, *net.UnixListener) {
	t.Helper()
	root := t.TempDir()
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	parts := []endpointDirectory{{"r", uid, gid, 0755}, {"b", uid, gid, 0755}, {"w", uid, gid, 0750}}
	name, mode := "worker.sock", os.FileMode(0660)
	if admin {
		parts[2].mode = 0700
		name, mode = "recovery.sock", 0600
	}
	path := root
	for _, part := range parts {
		path = filepath.Join(path, part.name)
		if os.Mkdir(path, os.FileMode(part.mode)) != nil || os.Chmod(path, os.FileMode(part.mode)) != nil {
			t.Fatal("owned directories")
		}
	}
	socket := filepath.Join(path, name)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal("owned socket")
	}
	listener.SetUnlinkOnClose(false)
	t.Cleanup(func() { listener.Close() })
	if os.Chmod(socket, mode) != nil {
		t.Fatal("socket mode")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("owned root")
	}
	p, err := pinEndpointAt(context.Background(), fd, parts, uid, gid, admin)
	if err != nil {
		t.Fatal("owned endpoint pin")
	}
	t.Cleanup(p.close)
	return p, socket, listener
}

func TestAdminEndpoint(t *testing.T) {
	for _, mode := range []string{"valid", "socket-mode", "leaf-mode", "peer"} {
		t.Run(mode, func(t *testing.T) {
			p, path, listener := ownedEndpointRole(t, true)
			if p.name != "recovery.sock" || p.mode != 0600 || p.directories[2].mode != 0700 {
				t.Fatal("admin used worker profile")
			}
			if mode == "socket-mode" && os.Chmod(path, 0660) != nil {
				t.Fatal("socket fixture")
			}
			if mode == "leaf-mode" && os.Chmod(filepath.Dir(path), 0750) != nil {
				t.Fatal("leaf fixture")
			}
			if mode == "socket-mode" || mode == "leaf-mode" {
				if p.check(context.Background()) == nil {
					t.Fatal("admin admitted worker permissions")
				}
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, err := connectPinned(ctx, p, path, func(*net.UnixConn) error {
				if mode == "peer" {
					return ErrProfile
				}
				return nil // Private synthetic peer; actual kernel proof is separate.
			})
			if mode == "peer" {
				if err == nil || c != nil {
					t.Fatal("admin accepted denied peer")
				}
			} else if err != nil || c == nil {
				t.Fatal("synthetic private admin connection refused")
			} else {
				c.Close()
			}
			listener.SetDeadline(time.Now().Add(time.Second))
			accepted, err := listener.AcceptUnix()
			if err != nil {
				t.Fatal("owned admin accept")
			}
			accepted.Close()
		})
	}
}

func TestNamedDialsRejectMissingOrCanceledContextBeforeProfileAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, dial := range []func(context.Context, Identity) (*net.UnixConn, error){DialWorkerBroker, DialAdminBroker} {
		for _, parent := range []context.Context{nil, ctx} {
			if c, err := dial(parent, Identity{}); err == nil || c != nil {
				t.Fatal("invalid named profile admitted")
			}
		}
	}
}

func TestWorkerEndpoint(t *testing.T) {
	for _, mode := range []string{"valid", "mode", "uid", "socket-link", "leaf-link", "ancestor", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			p, path, listener := ownedEndpoint(t)
			switch mode {
			case "mode":
				if os.Chmod(path, 0666) != nil {
					t.Fatal("fixture chmod")
				}
			case "uid":
				p.uid++
			case "socket-link":
				if os.Rename(path, path+".old") != nil || os.Symlink("worker.sock.old", path) != nil {
					t.Fatal("socket link fixture")
				}
			case "leaf-link":
				leaf := filepath.Dir(path)
				if os.Rename(leaf, leaf+".old") != nil || os.Symlink("w.old", leaf) != nil {
					t.Fatal("leaf link fixture")
				}
			case "ancestor":
				if unix.Fchmod(p.fds[1], 0777) != nil {
					t.Fatal("ancestor fixture")
				}
			case "replacement":
				if os.Rename(path, path+".old") != nil {
					t.Fatal("old socket")
				}
				other, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
				if err != nil {
					t.Fatal("replacement fixture")
				}
				t.Cleanup(func() { other.Close() })
				if os.Chmod(path, 0660) != nil {
					t.Fatal("replacement mode")
				}
			}
			err := p.check(context.Background())
			if mode == "valid" {
				if err != nil {
					t.Fatal("safe pin refused")
				}
				// Unrelated directory timestamps/link counts do not change its
				// protected identity. Creating another node must remain safe.
				if os.Mkdir(filepath.Join(filepath.Dir(path), "unrelated"), 0700) != nil || p.check(context.Background()) != nil {
					t.Fatal("directory timestamp treated as authority")
				}
			} else if err == nil {
				t.Fatal("unsafe endpoint accepted")
			}
			listener.Close()
		})
	}
}

func TestWorkerDial(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-peer", "change-after-dial", "canceled", "absent"} {
		t.Run(mode, func(t *testing.T) {
			p, path, listener := ownedEndpoint(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			if mode == "absent" {
				listener.Close()
			}
			var peerCalled bool
			peer := func(c *net.UnixConn) error {
				peerCalled = true
				raw, err := c.SyscallConn()
				if err != nil {
					return ErrProfile
				}
				matched := false
				if raw.Control(func(fd uintptr) {
					cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
					matched = err == nil && cred.Pid > 0 && cred.Uid == uint32(os.Geteuid()) && cred.Gid == uint32(os.Getegid())
				}) != nil || !matched {
					return ErrProfile
				}
				if mode == "wrong-peer" {
					return ErrProfile
				}
				if mode == "change-after-dial" && os.Chmod(path, 0666) != nil {
					t.Fatal("socket mutation fixture")
				}
				return nil
			}
			c, err := connectPinned(ctx, p, path, peer)
			if mode == "valid" {
				if err != nil || c == nil || !peerCalled {
					t.Fatal("kernel peer refused")
				}
				c.Close()
			} else {
				if err == nil || c != nil {
					t.Fatal("unsafe dial accepted")
				}
				if mode == "absent" && !errors.Is(err, ErrBrokerAbsent) {
					t.Fatal("refused broker not classified")
				}
				if mode == "canceled" && peerCalled {
					t.Fatal("canceled dial reached peer")
				}
			}
			if mode == "valid" || mode == "wrong-peer" || mode == "change-after-dial" {
				listener.SetDeadline(time.Now().Add(time.Second))
				accepted, err := listener.AcceptUnix()
				if err != nil {
					t.Fatal("owned accepted connection")
				}
				defer accepted.Close()
				accepted.SetReadDeadline(time.Now().Add(time.Second))
				var first [1]byte
				if n, err := accepted.Read(first[:]); n != 0 || err == nil {
					t.Fatal("dial emitted bytes before identity admission")
				}
			}
		})
	}
}
