//go:build linux

package connectedprofile

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestDockerActualIdentityBoundary(t *testing.T) {
	if len(os.Args) > 2 && os.Args[len(os.Args)-2] == "--docker-identity-child" {
		mode := os.Args[len(os.Args)-1]
		if mode != "no-nnp" && unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil {
			os.Exit(2)
		}
		c := dockerConfigFixture(t)
		var err error
		switch mode {
		case "collector":
			err = CheckRichProcessesDockerIdentity(c, true)
		case "reader":
			err = CheckRichProcessesDockerReaderIdentity(c)
		default:
			err = CheckRichProcessesDockerIdentity(c, false)
		}
		if err == nil {
			os.Exit(0)
		}
		os.Exit(1)
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		if os.Getenv("OBSERVER_CONNECTED_DOCKER_IDENTITY_ACCEPTANCE") == "1" {
			t.Fatal("identity fixture was not privileged")
		}
		t.Skip("root-owned child identity fixture")
	}
	c := dockerConfigFixture(t)
	for _, tc := range []struct {
		name, mode string
		uid, gid   uint32
		groups     []uint32
		ok         bool
	}{
		{"collector", "collector", c.CollectorUID, c.SharedGID, []uint32{c.FrameGID}, true},
		{"uploader", "uploader", c.UploaderUID, c.UploaderGID, []uint32{c.SharedGID}, true},
		{"reader", "reader", c.ReaderUID, c.FrameGID, []uint32{c.SocketGID}, true},
		{"reader-primary-also-listed", "reader", c.ReaderUID, c.FrameGID, []uint32{c.FrameGID, c.SocketGID}, true},
		{"reader-no-socket", "reader", c.ReaderUID, c.FrameGID, nil, false},
		{"collector-no-frame", "collector", c.CollectorUID, c.SharedGID, nil, false},
		{"uploader-no-handoff", "uploader", c.UploaderUID, c.UploaderGID, nil, false},
		{"uploader-socket-denied", "uploader", c.UploaderUID, c.UploaderGID, []uint32{c.SharedGID, c.SocketGID}, false},
		{"collector-socket-denied", "collector", c.CollectorUID, c.SharedGID, []uint32{c.FrameGID, c.SocketGID}, false},
		{"reader-handoff-denied", "reader", c.ReaderUID, c.FrameGID, []uint32{c.SocketGID, c.SharedGID}, false},
		{"wrong-uid", "reader", c.CollectorUID, c.FrameGID, []uint32{c.SocketGID}, false},
		{"no-elevation-fence", "no-nnp", c.UploaderUID, c.UploaderGID, []uint32{c.SharedGID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDockerActualIdentityBoundary$", "--", "--docker-identity-child", tc.mode)
			cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: tc.uid, Gid: tc.gid, Groups: tc.groups}}
			cmd.WaitDelay = time.Second
			err := cmd.Run()
			if ctx.Err() != nil || (err == nil) != tc.ok {
				t.Fatal("child identity boundary did not match expected authority")
			}
		})
	}
}
