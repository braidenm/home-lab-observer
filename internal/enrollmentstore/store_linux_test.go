//go:build linux

package enrollmentstore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
)

func directory(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
func setup(t *testing.T, path string) *Setup {
	t.Helper()
	s, err := OpenNew(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func through(t *testing.T, s *Setup, count int) {
	t.Helper()
	ctx := context.Background()
	b := testBinding()
	operations := []func() error{func() error { return s.Begin(ctx, b) }, func() error { return s.Create(ctx, enrollmentcoord.Credential{Binding: b, Secret: testSecret()}) }, func() error { return s.Provision(ctx, b) }, func() error { return s.MarkReady(ctx, b) }}
	for _, op := range operations[:count] {
		if err := op(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLifecycleAndAdvancedLedger(t *testing.T) {
	ctx := context.Background()
	path := directory(t)
	s := setup(t, path)
	through(t, s, 4)
	if _, err := OpenReady(ctx, path, testBinding()); err != ErrBusy {
		t.Fatalf("lease: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := OpenReady(ctx, path, testBinding())
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Credential()
	if err != nil || string(c.Secret) != string(testSecret()) {
		t.Fatal("credential mismatch")
	}
	c.Secret[0] = 'X'
	again, _ := r.Credential()
	if string(again.Secret) != string(testSecret()) {
		t.Fatal("alias")
	}
	old, err := r.Ledger().Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := old
	next.Watermark = 1
	if err := r.Ledger().Commit(ctx, old, next); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Credential(); err != ErrRecovery || r.Ledger() != nil {
		t.Fatal("closed ready usable")
	}
	r, err = OpenReady(ctx, path, testBinding())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, err := r.Ledger().Load(ctx)
	if err != nil || got.Watermark != 1 {
		t.Fatal("ledger reset")
	}
	for _, name := range []string{attemptName, readyName} {
		data, _ := os.ReadFile(filepath.Join(path, name))
		if strings.Contains(string(data), string(testSecret())) {
			t.Fatal("marker leaked secret")
		}
	}
}

func TestIncompleteNeverResumes(t *testing.T) {
	for step := 0; step < 4; step++ {
		t.Run(string(rune('0'+step)), func(t *testing.T) {
			path := directory(t)
			s := setup(t, path)
			through(t, s, step)
			s.Close()
			if _, err := OpenNew(context.Background(), path); err == nil {
				t.Fatal("reused partial root")
			}
			if r, err := OpenReady(context.Background(), path, testBinding()); err == nil {
				r.Close()
				t.Fatal("partial ready")
			}
		})
	}
}

func TestPublicationFaults(t *testing.T) {
	for _, point := range []string{"before_write", "after_write", "after_file_sync", "after_close", "after_rename", "after_directory_sync"} {
		for _, step := range []int{0, 1, 3} {
			t.Run(point+string(rune('0'+step)), func(t *testing.T) {
				path := directory(t)
				s := setup(t, path)
				through(t, s, step)
				s.hook = func(p string) error {
					if p == point {
						return errors.New("synthetic-private-canary")
					}
					return nil
				}
				ctx := context.Background()
				var err error
				switch step {
				case 0:
					err = s.Begin(ctx, testBinding())
				case 1:
					err = s.Create(ctx, enrollmentcoord.Credential{Binding: testBinding(), Secret: testSecret()})
				case 3:
					err = s.MarkReady(ctx, testBinding())
				}
				if err != ErrRecovery {
					t.Fatal("unfixed failure")
				}
				s.hook = nil
				if s.MarkReady(ctx, testBinding()) != ErrRecovery {
					t.Fatal("failed handle resumed")
				}
				s.Close()
				r, err := OpenReady(ctx, path, testBinding())
				witness := step == 3 && (point == "after_rename" || point == "after_directory_sync")
				if witness {
					if err != nil {
						t.Fatal(err)
					}
					r.Close()
				} else if err == nil {
					r.Close()
					t.Fatal("incomplete recovered")
				}
			})
		}
	}
}

func TestValidationAndMissingArtifacts(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{attemptName, credentialName, readyName, filepath.Join(ledgerName, "upload.sqlite")} {
		t.Run(name, func(t *testing.T) {
			path := directory(t)
			s := setup(t, path)
			through(t, s, 4)
			s.Close()
			if err := os.Remove(filepath.Join(path, name)); err != nil {
				t.Fatal(err)
			}
			if r, err := OpenReady(ctx, path, testBinding()); err == nil {
				r.Close()
				t.Fatal("missing artifact recovered")
			}
			if _, err := os.Stat(filepath.Join(path, name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("recreated missing state")
			}
		})
	}
	path := directory(t)
	s := setup(t, path)
	through(t, s, 4)
	s.Close()
	wrong := testBinding()
	wrong.ConnectorID = "agent_" + strings.Repeat("d", 32)
	if r, err := OpenReady(ctx, path, wrong); err == nil {
		r.Close()
		t.Fatal("wrong connector")
	}
	wrong = testBinding()
	wrong.ServerID = "srv_" + strings.Repeat("e", 32)
	if r, err := OpenReady(ctx, path, wrong); err == nil {
		r.Close()
		t.Fatal("wrong server")
	}
	if _, err := OpenNew(nil, directory(t)); err != ErrRecovery {
		t.Fatal("nil context")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := OpenNew(canceled, directory(t)); err != ErrRecovery {
		t.Fatal("canceled")
	}
}

func TestUnsafeEntriesAndRootReplacement(t *testing.T) {
	for _, kind := range []string{"mode", "symlink", "hardlink", "fifo", "oversize", "unknown", "acl", "default_acl"} {
		t.Run(kind, func(t *testing.T) {
			path := directory(t)
			s := setup(t, path)
			through(t, s, 4)
			s.Close()
			target := filepath.Join(path, credentialName)
			switch kind {
			case "mode":
				if err := os.Chmod(target, 0o644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				os.Remove(target)
				if err := os.Symlink("attempt.json", target); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, filepath.Join(t.TempDir(), "link")); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(target)
				if err := unix.Mkfifo(target, 0o600); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(target, make([]byte, maxRecord+1), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unknown":
				if err := os.WriteFile(filepath.Join(path, "other"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "acl", "default_acl":
				// Empty-access masks keep mode private while an extended ACL is present.
				acl := []byte{2, 0, 0, 0, 1, 0, 6, 0, 255, 255, 255, 255, 2, 0, 0, 0, 254, 255, 0, 0, 4, 0, 0, 0, 255, 255, 255, 255, 16, 0, 0, 0, 255, 255, 255, 255, 32, 0, 0, 0, 255, 255, 255, 255}
				name := "system.posix_acl_access"
				if kind == "default_acl" {
					target = path
					name = "system.posix_acl_default"
				}
				if err := unix.Setxattr(target, name, acl, 0); err != nil {
					t.Fatal(err)
				}
			}
			if r, err := OpenReady(context.Background(), path, testBinding()); err == nil {
				r.Close()
				t.Fatal("unsafe accepted")
			}
		})
	}
	parent := directory(t)
	path := filepath.Join(parent, "root")
	os.Mkdir(path, 0o700)
	s := setup(t, path)
	if err := os.Rename(path, filepath.Join(parent, "old")); err != nil {
		t.Fatal(err)
	}
	os.Mkdir(path, 0o700)
	if err := s.Begin(context.Background(), testBinding()); err != ErrRecovery {
		t.Fatal("replaced root")
	}
}

func TestInterruptedProcess(t *testing.T) {
	if path := os.Getenv("OBSERVER_ENROLLMENT_TEST_ROOT"); path != "" {
		s, err := OpenNew(context.Background(), path)
		if err != nil {
			os.Exit(8)
		}
		through(t, s, 3)
		point := os.Getenv("OBSERVER_ENROLLMENT_TEST_POINT")
		s.hook = func(p string) error {
			if p == point {
				os.Exit(7)
			}
			return nil
		}
		s.MarkReady(context.Background(), testBinding())
		os.Exit(9)
	}
	for _, point := range []string{"after_write", "after_file_sync", "after_rename", "after_directory_sync"} {
		t.Run(point, func(t *testing.T) {
			path := directory(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInterruptedProcess$")
			cmd.Env = []string{"OBSERVER_ENROLLMENT_TEST_ROOT=" + path, "OBSERVER_ENROLLMENT_TEST_POINT=" + point}
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 7 {
				t.Fatal("child did not reach checkpoint")
			}
			r, err := OpenReady(context.Background(), path, testBinding())
			if point == "after_rename" || point == "after_directory_sync" {
				if err != nil {
					t.Fatal(err)
				}
				r.Close()
			} else if err == nil {
				r.Close()
				t.Fatal("partial became ready")
			}
		})
	}
}

func TestSequencingCancellationAndIndependentLedgerCheck(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"wrong-order", "wrong-binding", "invalid-secret", "canceled", "nil-context", "repeat-begin"} {
		t.Run(bad, func(t *testing.T) {
			s := setup(t, directory(t))
			var err error
			switch bad {
			case "wrong-order":
				err = s.MarkReady(ctx, testBinding())
			case "wrong-binding":
				through(t, s, 1)
				b := testBinding()
				b.ConnectorID = "agent_" + strings.Repeat("d", 32)
				err = s.Create(ctx, enrollmentcoord.Credential{Binding: b, Secret: testSecret()})
			case "invalid-secret":
				through(t, s, 1)
				err = s.Create(ctx, enrollmentcoord.Credential{Binding: testBinding(), Secret: []byte("private-canary")})
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				err = s.Begin(c, testBinding())
			case "nil-context":
				err = s.Begin(nil, testBinding())
			case "repeat-begin":
				through(t, s, 1)
				err = s.Begin(ctx, testBinding())
			}
			if err != ErrRecovery || s.Begin(ctx, testBinding()) != ErrRecovery {
				t.Fatal("failure did not latch")
			}
		})
	}
	path := directory(t)
	s := setup(t, path)
	through(t, s, 3)
	l, err := uploadledger.OpenExisting(ctx, filepath.Join(path, ledgerName), testBinding())
	if err != nil {
		t.Fatal(err)
	}
	old, err := l.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := old
	next.Watermark = 1
	if err := l.Commit(ctx, old, next); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkReady(ctx, testBinding()); err != ErrRecovery {
		t.Fatal("nonpristine ledger marked ready")
	}
	if _, err := os.Stat(filepath.Join(path, readyName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("published ready")
	}
}

func TestCorruptFilesAndNoReplace(t *testing.T) {
	for _, name := range []string{attemptName, credentialName, readyName, filepath.Join(ledgerName, "upload.sqlite")} {
		t.Run(name, func(t *testing.T) {
			path := directory(t)
			s := setup(t, path)
			through(t, s, 4)
			s.Close()
			canary := []byte("private-malformed-canary")
			if err := os.WriteFile(filepath.Join(path, name), canary, 0o600); err != nil {
				t.Fatal(err)
			}
			if r, err := OpenReady(context.Background(), path, testBinding()); err == nil {
				r.Close()
				t.Fatal("corruption recovered")
			}
			got, err := os.ReadFile(filepath.Join(path, name))
			if err != nil || string(got) != string(canary) {
				t.Fatal("corruption repaired")
			}
		})
	}
	path := directory(t)
	s := setup(t, path)
	if err := s.publish(attemptName, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := s.publish(attemptName, []byte("second")); err != ErrRecovery {
		t.Fatal("overwrote")
	}
	got, _ := os.ReadFile(filepath.Join(path, attemptName))
	if string(got) != "first" {
		t.Fatal("replaced immutable record")
	}
}

func TestExclusiveLeaseAcrossProcesses(t *testing.T) {
	if path := os.Getenv("OBSERVER_ENROLLMENT_LEASE_TEST_ROOT"); path != "" {
		r, err := OpenReady(context.Background(), path, testBinding())
		if r != nil {
			r.Close()
		}
		if err != ErrBusy {
			os.Exit(8)
		}
		return
	}
	ctx := context.Background()
	path := directory(t)
	s := setup(t, path)
	through(t, s, 4)
	for _, holder := range []string{"setup", "ready"} {
		t.Run(holder, func(t *testing.T) {
			if holder == "ready" {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				r, err := OpenReady(ctx, path, testBinding())
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
			}
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(bounded, os.Args[0], "-test.run=^TestExclusiveLeaseAcrossProcesses$")
			cmd.Env = []string{"OBSERVER_ENROLLMENT_LEASE_TEST_ROOT=" + path}
			if cmd.Run() != nil {
				t.Fatal("second process did not refuse held lease")
			}
		})
	}
}
