//go:build linux

package actionworker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func credentialStoreFixture(t *testing.T) (*credentialStore, string) {
	t.Helper()
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil || os.Mkdir(filepath.Join(root, "worker"), 0700) != nil {
		t.Fatal("private directory")
	}
	b, _, _ := renewalStateFixture(t)
	raw, _ := encodeHandoff(freshHandoff(b), b)
	for name, data := range map[string][]byte{"credential.lock": {}, "handoff.json": raw} {
		if os.WriteFile(filepath.Join(root, "worker", name), data, 0600) != nil {
			t.Fatal("private store")
		}
	}
	open := func() (*credentialStore, error) {
		fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal("fixture root")
		}
		return openCredentialStoreAt(context.Background(), fd, []protectedPart{{"worker", uint32(os.Geteuid()), uint32(os.Getegid()), 0700}}, b)
	}
	s, err := open()
	if err != nil {
		t.Fatal("store open")
	}
	t.Cleanup(func() { s.close() })
	if other, err := open(); err == nil {
		other.close()
		t.Fatal("second writer admitted")
	}
	return s, root
}

func TestEnrollmentSeedOnlyPublishesIntoEmptyStore(t *testing.T) {
	for _, mode := range []string{"empty", "existing", "appears-before-rename"} {
		t.Run(mode, func(t *testing.T) {
			s, root := credentialStoreFixture(t)
			path := filepath.Join(root, "worker", "handoff.json")
			if mode != "existing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "appears-before-rename" {
				s.fault = func(stage string) error {
					if stage == "before-rename" {
						return os.WriteFile(path, []byte("retained"), 0600)
					}
					return nil
				}
			}
			op, err := newCredentialOperation(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer op.close()
			if err := s.seed(op); (err == nil) != (mode == "empty") {
				t.Fatal("unexpected seed result")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "appears-before-rename" {
				if string(raw) != "retained" {
					t.Fatal("seed replaced retained state")
				}
			} else if h, err := decodeHandoff(raw, s.binding); err != nil || h.Phase != phaseUnenrolled {
				t.Fatal("invalid initial state")
			}
		})
	}
}

func TestCredentialStoreLeaseCASAndDurableAttempt(t *testing.T) {
	s, root := credentialStoreFixture(t)
	op, err := newCredentialOperation(context.Background())
	if err != nil {
		t.Fatal("operation")
	}
	defer op.close()
	h, _, err := s.load(op.ctx)
	if err != nil || h.Phase != phaseUnenrolled {
		t.Fatal("fresh load")
	}
	h.Phase = phaseExchangeAttempted
	if s.handoff(op, h) != nil {
		t.Fatal("durable attempt")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "worker", "handoff.json"))
	loaded, err := decodeHandoff(raw, s.binding)
	if err != nil || loaded.Phase != phaseExchangeAttempted {
		t.Fatal("attempt not retained")
	}
	// Same bytes on a replacement inode are not the expected CAS witness.
	if os.WriteFile(filepath.Join(root, "worker", "replacement"), raw, 0600) != nil || os.Rename(filepath.Join(root, "worker", "replacement"), filepath.Join(root, "worker", "handoff.json")) != nil {
		t.Fatal("fixture replace")
	}
	if s.handoff(op, h) == nil {
		t.Fatal("replaced CAS admitted")
	}
}

func TestCredentialStorePostRenameFailurePreservesAttemptAndPoisonsOwner(t *testing.T) {
	s, root := credentialStoreFixture(t)
	op, _ := newCredentialOperation(context.Background())
	defer op.close()
	h, _, err := s.load(op.ctx)
	if err != nil {
		t.Fatal("load")
	}
	h.Phase = phaseExchangeAttempted
	s.fault = func(stage string) error {
		if stage == "after-rename" {
			return ErrUnsafe
		}
		return nil
	}
	if s.handoff(op, h) == nil {
		t.Fatal("ambiguous publication accepted")
	}
	if _, _, err := s.load(op.ctx); err == nil {
		t.Fatal("poisoned owner reused")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "worker", "handoff.json"))
	retained, err := decodeHandoff(raw, s.binding)
	if err != nil || retained.Phase != phaseExchangeAttempted {
		t.Fatal("actual rename falsely rolled back")
	}
}

func TestCredentialStoreRejectsLockReplacementAndLateMutation(t *testing.T) {
	s, root := credentialStoreFixture(t)
	op, _ := newCredentialOperation(context.Background())
	h, _, _ := s.load(op.ctx)
	op.close()
	if s.handoff(op, h) == nil {
		t.Fatal("canceled publication accepted")
	}
	if os.Rename(filepath.Join(root, "worker", "credential.lock"), filepath.Join(root, "worker", "saved.lock")) != nil || os.WriteFile(filepath.Join(root, "worker", "credential.lock"), nil, 0600) != nil {
		t.Fatal("lock replacement")
	}
	if _, _, err := s.load(context.Background()); err == nil {
		t.Fatal("foreign lock adopted")
	}
}

func TestCredentialLeaseChild(t *testing.T) {
	root := os.Getenv("HLO_SYNTHETIC_LEASE_ROOT")
	if root == "" {
		return
	}
	b, _, _ := renewalStateFixture(t)
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("private child fixture")
	}
	s, err := openCredentialStoreAt(context.Background(), fd, []protectedPart{{"worker", uint32(os.Geteuid()), uint32(os.Getegid()), 0700}}, b)
	if err == nil {
		s.close()
		t.Fatal("second process acquired live lease")
	}
}
func TestCredentialStoreLeaseExcludesASeparateProcess(t *testing.T) {
	_, root := credentialStoreFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCredentialLeaseChild$", "-test.timeout=4s")
	cmd.Env = append(os.Environ(), "HLO_SYNTHETIC_LEASE_ROOT="+root)
	if output, err := cmd.CombinedOutput(); err != nil {
		clear(output)
		t.Fatal("separate process lease refusal failed")
	}
}

func TestCredentialStoreCancellationAfterFileSyncDoesNotRename(t *testing.T) {
	s, root := credentialStoreFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op, _ := newCredentialOperation(ctx)
	defer op.close()
	h, _, _ := s.load(ctx)
	h.Phase = phaseExchangeAttempted
	s.fault = func(stage string) error {
		if stage == "after-file-sync" {
			cancel()
		}
		return nil
	}
	if s.handoff(op, h) == nil {
		t.Fatal("canceled publication accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "worker", "handoff.json"))
	defer clear(raw)
	retained, err := decodeHandoff(raw, s.binding)
	if err != nil || retained.Phase != phaseUnenrolled {
		t.Fatal("canceled pre-rename write replaced record")
	}
}
