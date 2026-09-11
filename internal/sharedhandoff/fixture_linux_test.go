//go:build linux

package sharedhandoff

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

var fixturePolicy = Policy{CollectorUID: 61001, SharedGID: 61011, UploaderUID: 61002, ServerID: "srv_0123456789abcdef0123456789abcdef"}

func sample() (observation.Snapshot, remoteprojection.Identity) {
	return observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)}, remoteprojection.Identity{SourceID: fixturePolicy.ServerID, Version: "0.1.0-preview.3", OS: "linux"}
}

// This explicitly opted-in fixture changes ownership only under its freshly
// created private temporary tree. No host users, groups, services or mounts.
func TestOwnedCrossPrincipalFixture(t *testing.T) {
	if role := os.Getenv("OBSERVER_SHARED_CHILD"); role != "" {
		runChild(t, role)
		return
	}
	if os.Getenv("OBSERVER_SHARED_ACCEPTANCE") != "1" {
		t.Skip("owned root DAC/ACL fixture not requested")
	}
	if os.Geteuid() != 0 {
		t.Fatal("owned fixture requires root orchestrator")
	}
	if configured := os.Getenv("TMPDIR"); configured != "" && configured != "/tmp" {
		t.Fatal("fixture requires fixed native tmpdir")
	}
	tmpInfo, err := os.Lstat("/tmp")
	if err != nil || !tmpInfo.IsDir() || tmpInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("fixture tmpdir is not a real directory")
	}
	outer := t.TempDir()
	resolved, err := filepath.EvalSymlinks(outer)
	if err != nil || resolved != filepath.Clean(outer) || !strings.HasPrefix(resolved, "/tmp/") {
		t.Fatal("fixture must use native temporary root")
	}
	if err := os.Chmod(outer, 0o700); err != nil {
		t.Fatal("fixture outer mode failed")
	}
	root := filepath.Join(outer, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal("fixture root creation failed")
	}
	if err := os.WriteFile(filepath.Join(root, "fixture-marker"), []byte("observer-shared-owned-fixture/v1"), 0o444); err != nil {
		t.Fatal("fixture marker creation failed")
	}
	state := filepath.Join(root, "handoff")
	if err := os.Mkdir(state, 0o750); err != nil {
		t.Fatal("fixture state creation failed")
	}
	if err := os.Chown(state, int(fixturePolicy.CollectorUID), int(fixturePolicy.SharedGID)); err != nil {
		t.Fatal("fixture ownership failed")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal("fixture executable unavailable")
	}
	in, err := os.Open(self)
	if err != nil {
		t.Fatal("fixture executable unavailable")
	}
	info, err := in.Stat()
	if err != nil || info.Size() > 200*1024*1024 {
		in.Close()
		t.Fatal("fixture executable size")
	}
	out, err := os.OpenFile(filepath.Join(root, "helper"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o555)
	if err != nil {
		in.Close()
		t.Fatal("fixture executable stage failed")
	}
	n, copyErr := io.Copy(out, io.LimitReader(in, 200*1024*1024+1))
	in.Close()
	out.Close()
	if copyErr != nil || n != info.Size() {
		t.Fatal("fixture executable copy failed")
	}
	for _, role := range []string{"reader-missing", "publish", "read-denials", "third", "wrong-reader", "wrong-group", "writer-wrong-group", "writer-tests"} {
		runOwned(t, root, role)
	}
	runConcurrent(t, root)
	// All mutations below affect only synthetic files in this owned chroot.
	for _, kind := range []string{"access", "default"} {
		name := "system.posix_acl_" + kind
		if err := unix.Setxattr(state, name, fixtureACL(true), 0); err != nil {
			t.Fatal("fixture ACL setup failed")
		}
		runOwned(t, root, "reject-directory")
		if err := unix.Removexattr(state, name); err != nil {
			t.Fatal("fixture ACL removal failed")
		}
	}
	path := filepath.Join(state, latestName)
	if err := unix.Setxattr(path, "system.posix_acl_access", fixtureACL(false), 0); err != nil {
		t.Fatal("fixture file ACL setup failed")
	}
	runOwned(t, root, "reject-file")
	if err := unix.Removexattr(path, "system.posix_acl_access"); err != nil {
		t.Fatal("fixture file ACL removal failed")
	}
	runOwned(t, root, "read-denials")
	if err := os.WriteFile(path, []byte("synthetic-invalid-document"), 0o640); err != nil {
		t.Fatal("invalid document setup failed")
	}
	runOwned(t, root, "read-invalid")
	runOwned(t, root, "publish")
	t.Log("SHARED_HANDOFF_DAC_ACL_PASS")
}

func fixtureACL(directory bool) []byte {
	// Linux POSIX ACL xattr v2, fixed synthetic named third-user grant. Mask
	// matches the normal group bits, so mode-only validation cannot detect it.
	group := uint16(4)
	owner := uint16(6)
	if directory {
		group = 5
		owner = 7
	}
	entries := []struct {
		tag, perm uint16
		id        uint32
	}{{1, owner, ^uint32(0)}, {2, group, 61003}, {4, group, ^uint32(0)}, {16, group, ^uint32(0)}, {32, 0, ^uint32(0)}}
	b := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(b, 2)
	for i, e := range entries {
		pos := 4 + i*8
		binary.LittleEndian.PutUint16(b[pos:], e.tag)
		binary.LittleEndian.PutUint16(b[pos+2:], e.perm)
		binary.LittleEndian.PutUint32(b[pos+4:], e.id)
	}
	return b
}

type cappedOutput struct {
	n        int
	overflow bool
}

func (w *cappedOutput) Write(b []byte) (int, error) {
	w.n += len(b)
	if w.n > 4096 {
		w.overflow = true
		return 0, errors.New("fixture_output_limit")
	}
	return len(b), nil
}

func runOwned(t *testing.T, root, role string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uid, gid, groups := fixturePolicy.CollectorUID, fixturePolicy.SharedGID, []uint32{}
	if role == "read-denials" || role == "reject-directory" || role == "reject-file" || role == "read-loop" || role == "reader-missing" || role == "read-invalid" {
		uid, gid, groups = fixturePolicy.UploaderUID, 61012, []uint32{fixturePolicy.SharedGID}
	}
	if role == "third" || role == "wrong-reader" {
		uid, gid = 61003, 61013
	}
	if role == "wrong-group" {
		uid, gid, groups = fixturePolicy.UploaderUID, 61012, []uint32{}
	}
	if role == "writer-wrong-group" {
		uid, gid, groups = fixturePolicy.CollectorUID, 61012, []uint32{}
	}
	cmd := exec.CommandContext(ctx, "/helper", "-test.run=^TestOwnedCrossPrincipalFixture$", "-test.timeout=8s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root, Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: groups}}
	cmd.Dir = "/"
	cmd.Env = []string{"OBSERVER_SHARED_CHILD=" + role}
	cmd.Stdin = bytes.NewReader(nil)
	output := &cappedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil || output.overflow {
		t.Fatalf("owned fixture child failed: %s", role)
	}
}

func runChild(t *testing.T, role string) {
	if os.Geteuid() == 0 {
		t.Fatal("child retained root")
	}
	marker, err := os.ReadFile("/fixture-marker")
	if err != nil || string(marker) != "observer-shared-owned-fixture/v1" {
		t.Fatal("child is not in owned fixture")
	}
	var caps [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if unix.Capget(&header, &caps[0]) != nil {
		t.Fatal("child capabilities unavailable")
	}
	for _, c := range caps {
		if c.Effective != 0 || c.Permitted != 0 || c.Inheritable != 0 {
			t.Fatal("child retained capabilities")
		}
	}
	p := fixturePolicy
	switch role {
	case "publish":
		w, err := OpenWriter("/handoff", p)
		if err != nil {
			t.Fatal("writer open failed")
		}
		defer w.Close()
		raw, id := sample()
		if err := w.Publish(raw, id); err != nil {
			t.Fatal("publish failed")
		}
	case "read-denials":
		r, err := OpenReader("/handoff", p)
		if err != nil {
			t.Fatal("reader open failed")
		}
		defer r.Close()
		if data, err := r.Read(nil, p.ServerID); data != nil || err != ErrUnavailable {
			t.Fatal("nil context not fixed failure")
		}
		if data, err := r.Read(context.Background(), "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); data != nil || err != ErrInvalid {
			t.Fatal("wrong source read accepted")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if data, err := r.Read(ctx, p.ServerID); data != nil || err != ErrUnavailable {
			t.Fatal("cancelled read accepted")
		}
		data, err := r.Read(context.Background(), p.ServerID)
		if err != nil || remoteprojection.Validate(data, p.ServerID) != nil {
			t.Fatal("reader failed")
		}
		for _, path := range []string{"/handoff/snapshot.json", "/handoff/new-file", "/handoff/.writer-lock"} {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
			if err == nil {
				f.Close()
				t.Fatal("reader gained write")
			}
			if !errors.Is(err, os.ErrPermission) {
				t.Fatal("write denial was not permission")
			}
		}
		if err := os.Remove("/handoff/snapshot.json"); !errors.Is(err, os.ErrPermission) {
			t.Fatal("reader remove not denied")
		}
		if err := os.Rename("/handoff/snapshot.json", "/handoff/replaced"); !errors.Is(err, os.ErrPermission) {
			t.Fatal("reader rename not denied")
		}
		if _, err := OpenWriter("/handoff", p); err != ErrUnsafe {
			t.Fatal("reader obtained writer API")
		}
		if r.Close() != nil || r.Close() != nil {
			t.Fatal("reader close failed")
		}
		if data, err := r.Read(context.Background(), p.ServerID); data != nil || err != ErrUnavailable {
			t.Fatal("closed reader accepted")
		}
	case "reader-missing", "read-invalid":
		r, err := OpenReader("/handoff", p)
		if err != nil {
			t.Fatal("negative reader open failed")
		}
		defer r.Close()
		want := ErrInvalid
		if role == "reader-missing" {
			want = ErrMissing
		}
		if data, err := r.Read(context.Background(), p.ServerID); data != nil || err != want {
			t.Fatal("negative read not refused")
		}
	case "third":
		if _, err := os.ReadFile("/handoff/snapshot.json"); !errors.Is(err, os.ErrPermission) {
			t.Fatal("third user read not denied")
		}
	case "wrong-reader":
		if _, err := OpenReader("/handoff", p); err != ErrUnsafe {
			t.Fatal("wrong user accepted")
		}
	case "wrong-group":
		if r, err := OpenReader("/handoff", p); err != ErrUnsafe {
			if r != nil {
				r.Close()
			}
			t.Fatal("wrong group accepted")
		}
	case "writer-wrong-group":
		if w, err := OpenWriter("/handoff", p); err != ErrUnsafe {
			if w != nil {
				w.Close()
			}
			t.Fatal("wrong writer group accepted")
		}
	case "write-loop":
		w, err := OpenWriter("/handoff", p)
		if err != nil {
			t.Fatal("loop writer failed")
		}
		defer w.Close()
		raw, id := sample()
		fmt.Fprintln(os.Stdout, "READY")
		for i := 0; i < 250; i++ {
			raw.ObservedAt = raw.ObservedAt.Add(time.Second)
			if err := w.Publish(raw, id); err != nil {
				t.Fatal("loop publication failed")
			}
		}
	case "read-loop":
		r, err := OpenReader("/handoff", p)
		if err != nil {
			t.Fatal("loop reader failed")
		}
		defer r.Close()
		for i := 0; i < 1000; i++ {
			data, err := r.Read(context.Background(), p.ServerID)
			if err == ErrUnavailable {
				continue
			}
			if err != nil || remoteprojection.Validate(data, p.ServerID) != nil {
				t.Fatal("loop returned unsafe document")
			}
		}
	case "reject-directory":
		if r, err := OpenReader("/handoff", p); err != ErrUnsafe {
			if r != nil {
				r.Close()
			}
			t.Fatal("directory ACL accepted")
		}
	case "reject-file":
		r, err := OpenReader("/handoff", p)
		if err != nil {
			t.Fatal("reader fixture open failed")
		}
		defer r.Close()
		if data, err := r.Read(context.Background(), p.ServerID); data != nil || err != ErrUnsafe {
			t.Fatal("file ACL accepted")
		}
	case "writer-tests":
		writerTests(t)
	default:
		t.Fatal("unknown fixture role")
	}
}

type readyOutput struct {
	cappedOutput
	prefix   []byte
	ready    chan struct{}
	notified bool
}

func (w *readyOutput) Write(b []byte) (int, error) {
	if !w.notified {
		needed := 6 - len(w.prefix)
		if needed > len(b) {
			needed = len(b)
		}
		w.prefix = append(w.prefix, b[:needed]...)
		if bytes.Equal(w.prefix, []byte("READY\n")) {
			w.notified = true
			close(w.ready)
		}
	}
	return w.cappedOutput.Write(b)
}

func runConcurrent(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	cmd := exec.CommandContext(ctx, "/helper", "-test.run=^TestOwnedCrossPrincipalFixture$", "-test.timeout=12s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: root, Credential: &syscall.Credential{Uid: fixturePolicy.CollectorUID, Gid: fixturePolicy.SharedGID, Groups: []uint32{}}}
	cmd.Dir = "/"
	cmd.Env = []string{"OBSERVER_SHARED_CHILD=write-loop"}
	cmd.Stdin = bytes.NewReader(nil)
	output := &readyOutput{ready: make(chan struct{})}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal("concurrent fixture start failed")
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	defer func() { cancel(); <-done }()
	select {
	case <-output.ready:
	case <-done:
		t.Fatal("concurrent fixture readiness failed")
	case <-ctx.Done():
		t.Fatal("concurrent fixture readiness timeout")
	}
	runOwned(t, root, "read-loop")
	<-done
	if waitErr != nil || output.overflow {
		t.Fatal("concurrent fixture failed")
	}
}

func writerTests(t *testing.T) {
	w, err := OpenWriter("/handoff", fixturePolicy)
	if err != nil {
		t.Fatal("writer test open failed")
	}
	defer w.Close()
	if second, err := OpenWriter("/handoff", fixturePolicy); err != ErrBusy {
		if second != nil {
			second.Close()
		}
		t.Fatal("second writer accepted")
	}
	raw, id := sample()
	wrong := id
	wrong.SourceID = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := w.Publish(raw, wrong); err != ErrInvalid {
		t.Fatal("wrong source accepted")
	}
	w.s.writeFile = func(f *os.File, b []byte) (int, error) { return f.Write(b[:len(b)/2]) }
	if err := w.Publish(raw, id); err != ErrUnavailable {
		t.Fatal("partial write accepted")
	}
	w.s.writeFile = (*os.File).Write
	if err := w.Publish(raw, id); err != nil {
		t.Fatal("staging recovery failed")
	}
	for _, fault := range []string{"sync-file", "rename", "sync-dir"} {
		errFixture := errors.New("synthetic_failure")
		switch fault {
		case "sync-file":
			w.s.syncFile = func(*os.File) error { return errFixture }
		case "rename":
			w.s.replace = func(string, string) error { return errFixture }
		case "sync-dir":
			w.s.syncDir = func(*os.File) error { return errFixture }
		}
		if err := w.Publish(raw, id); err != ErrUnavailable {
			t.Fatal("write fault accepted")
		}
		w.s.syncFile = (*os.File).Sync
		w.s.syncDir = (*os.File).Sync
		w.s.replace = w.s.root.Rename
		if err := w.Publish(raw, id); err != nil {
			t.Fatal("fault recovery failed")
		}
	}
	for _, fault := range []string{"unknown", "symlink", "hardlink", "mode", "oversize"} {
		original, err := os.ReadFile("/handoff/snapshot.json")
		if err != nil {
			t.Fatal("fixture baseline read failed")
		}
		switch fault {
		case "unknown":
			err = os.WriteFile("/handoff/unexpected", nil, 0o600)
		case "symlink":
			err = os.Symlink("/absent", "/handoff/.snapshot-next")
		case "hardlink":
			err = os.Link("/handoff/snapshot.json", "/handoff/.snapshot-next")
		case "mode":
			err = os.Chmod("/handoff/snapshot.json", 0o660)
		case "oversize":
			err = os.WriteFile("/handoff/snapshot.json", make([]byte, remoteprojection.MaxBytes+1), 0o640)
		}
		if err != nil {
			t.Fatal("unsafe fixture setup failed")
		}
		if err := w.Publish(raw, id); err != ErrUnsafe {
			t.Fatal("unsafe existing entry accepted")
		}
		switch fault {
		case "unknown":
			err = os.Remove("/handoff/unexpected")
		case "symlink", "hardlink":
			err = os.Remove("/handoff/.snapshot-next")
		case "mode":
			err = os.Chmod("/handoff/snapshot.json", 0o640)
		case "oversize":
			err = os.WriteFile("/handoff/snapshot.json", original, 0o640)
		}
		if err != nil {
			t.Fatal("owned fixture restore failed")
		}
	}
	if w.Close() != nil || w.Close() != nil {
		t.Fatal("writer close failed")
	}
	if w.Publish(raw, id) != ErrUnavailable {
		t.Fatal("closed writer accepted")
	}
}
