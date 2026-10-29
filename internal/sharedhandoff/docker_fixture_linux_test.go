//go:build linux

package sharedhandoff

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Extends the existing opt-in isolated chroot fixture. IDs are synthetic; only
// newly created test directories are changed. No Docker daemon or host account.
func runDockerFrameFixture(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "docker-frame")
	if os.Mkdir(dir, 0750) != nil || os.Chown(dir, int(fixturePolicy.CollectorUID), int(fixturePolicy.SharedGID)) != nil {
		t.Fatal("frame fixture setup")
	}
	for _, role := range []string{"frame-read-missing", "frame-publish", "frame-read", "frame-third", "frame-writer-tests", "frame-publish-large", "frame-read-large", "frame-publish"} {
		runOwned(t, root, role)
	}
	path := filepath.Join(dir, latestName)
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		if unix.Setxattr(dir, name, fixtureACL(true), 0) != nil {
			t.Fatal("frame directory ACL fixture")
		}
		runOwned(t, root, "frame-read-reject-open")
		if unix.Removexattr(dir, name) != nil {
			t.Fatal("frame directory ACL cleanup")
		}
	}
	if unix.Setxattr(path, "system.posix_acl_access", fixtureACL(false), 0) != nil {
		t.Fatal("frame file ACL fixture")
	}
	runOwned(t, root, "frame-read-reject-file")
	if unix.Removexattr(path, "system.posix_acl_access") != nil {
		t.Fatal("frame file ACL cleanup")
	}
	if os.Chmod(path, 0660) != nil {
		t.Fatal("frame mode fixture")
	}
	runOwned(t, root, "frame-read-reject-file")
	if os.Chmod(path, 0640) != nil {
		t.Fatal("frame mode cleanup")
	}
	link := filepath.Join(root, "frame-extra-link")
	if os.Link(path, link) != nil {
		t.Fatal("frame hardlink fixture")
	}
	runOwned(t, root, "frame-read-reject-file")
	if os.Remove(link) != nil {
		t.Fatal("frame hardlink cleanup")
	}
	if os.Remove(path) != nil || os.Symlink("/fixture-marker", path) != nil {
		t.Fatal("frame symlink fixture")
	}
	runOwned(t, root, "frame-read-reject-file")
	if os.Remove(path) != nil {
		t.Fatal("frame symlink cleanup")
	}
	runOwned(t, root, "frame-publish")
	runOwned(t, root, "frame-read")
}

func runDockerFrameChild(t *testing.T, role string) {
	p, section, at := frameFixture(t)
	if role == "frame-publish-large" || role == "frame-read-large" {
		p, section, at = largeFrameFixture(t)
	}
	if role == "frame-third" {
		if _, err := os.ReadFile("/docker-frame/" + latestName); !errors.Is(err, os.ErrPermission) {
			t.Fatal("unrelated/uploader principal read frame")
		}
		if r, err := OpenDockerFrameReader("/docker-frame", p); err == nil {
			r.Close()
			t.Fatal("unrelated principal gained reader API")
		}
		return
	}
	if role == "frame-publish" || role == "frame-publish-large" || role == "frame-writer-tests" {
		w, err := OpenDockerFrameWriter("/docker-frame", p)
		if err != nil {
			t.Fatal("frame writer refused")
		}
		defer w.Close()
		if role == "frame-writer-tests" {
			if other, err := OpenDockerFrameWriter("/docker-frame", p); err == nil {
				other.Close()
				t.Fatal("second writer admitted")
			}
			if err := w.Publish(section, time.Time{}); err != ErrInvalid {
				t.Fatal("invalid publication accepted")
			}
		}
		if w.Publish(section, at) != nil {
			t.Fatal("frame publish refused")
		}
		if role == "frame-writer-tests" {
			if w.Close() != nil || w.Close() != nil {
				t.Fatal("frame close not idempotent")
			}
			if w.Publish(section, at) != ErrUnavailable {
				t.Fatal("closed frame writer accepted")
			}
		}
		return
	}
	r, err := OpenDockerFrameReader("/docker-frame", p)
	if role == "frame-read-reject-file" && err == ErrUnsafe {
		return // Metadata (mode/link/type) may already fail the open audit.
	}
	if role == "frame-read-reject-open" {
		if err == nil {
			r.Close()
			t.Fatal("unsafe frame directory accepted")
		}
		return
	}
	if err != nil {
		t.Fatal("frame reader refused")
	}
	defer r.Close()
	s, err := r.Read(context.Background(), p.ServerID, at)
	if role == "frame-read-missing" {
		if err != ErrMissing {
			t.Fatal("missing frame not distinguished")
		}
		return
	}
	if role == "frame-read-reject-file" {
		if err == nil {
			t.Fatal("unsafe frame accepted")
		}
		return
	}
	want, _ := section.Bytes()
	got, _ := s.Bytes()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("frame changed")
	}
	if _, err := r.Read(context.Background(), p.ServerID, at.Add(46*time.Second)); err != ErrInvalid {
		t.Fatal("stale frame accepted")
	}
	if _, err := r.Read(context.Background(), p.ServerID, at.Add(-6*time.Second)); err != ErrInvalid {
		t.Fatal("future frame accepted")
	}
	wrong := p
	wrong.PolicyDigest[0]++
	other, err := OpenDockerFrameReader("/docker-frame", wrong)
	if err != nil {
		t.Fatal("negative policy fixture open")
	}
	if _, err := other.Read(context.Background(), p.ServerID, at); err != ErrInvalid {
		t.Fatal("foreign policy accepted")
	}
	other.Close()
	for _, name := range []string{latestName, stageName, lockName} {
		f, err := os.OpenFile("/docker-frame/"+name, os.O_WRONLY|os.O_CREATE, 0600)
		if err == nil {
			f.Close()
			t.Fatal("collector gained frame write")
		}
	}
	if w, err := OpenDockerFrameWriter("/docker-frame", p); err == nil {
		w.Close()
		t.Fatal("collector gained writer role")
	}
	if r.Close() != nil || r.Close() != nil {
		t.Fatal("frame reader close failed")
	}
	if _, err := r.Read(context.Background(), p.ServerID, at); err != ErrUnavailable {
		t.Fatal("closed reader accepted")
	}
}
