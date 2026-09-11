package handoff

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/ownerfs"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

const serverID = "srv_0123456789abcdef0123456789abcdef"

func privateDirectory(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setFixtureDirectoryOwner(t, dir)
	if err := ownerfs.RestrictDirectory(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sample() (observation.Snapshot, remoteprojection.Identity) {
	// All sections unavailable is a valid disposable host projection.
	return observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)}, remoteprojection.Identity{SourceID: serverID, Version: "0.1.0-preview.3", OS: "linux"}
}

func openStore(t *testing.T, dir string, writable bool) *Store {
	t.Helper()
	s, err := Open(dir, writable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPublishReadAndWriterExclusion(t *testing.T) {
	dir := privateDirectory(t)
	w := openStore(t, dir, true)
	r := openStore(t, dir, false)
	if data, err := r.Read(serverID); data != nil || !errors.Is(err, ErrMissing) {
		t.Fatal("missing slot not reported")
	}
	if duplicate, err := Open(dir, true); !errors.Is(err, ErrBusy) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatal("second writer accepted")
	}
	raw, identity := sample()
	if err := r.Publish(raw, identity); !errors.Is(err, ErrUnavailable) {
		t.Fatal("reader published")
	}
	if err := w.Publish(raw, identity); err != nil {
		t.Fatal(err)
	}
	got, err := r.Read(serverID)
	want, _ := remoteprojection.Encode(raw, identity)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("published bytes differ")
	}
	if data, err := r.Read("srv_ffffffffffffffffffffffffffffffff"); data != nil || !errors.Is(err, ErrInvalid) {
		t.Fatal("cross-server read accepted")
	}
	if w.Close() != nil || w.Close() != nil {
		t.Fatal("close not idempotent")
	}
	openStore(t, dir, true)
	if w.Publish(raw, identity) != ErrUnavailable {
		t.Fatal("closed writer accepted publish")
	}
	if data, err := w.Read(serverID); data != nil || err != ErrUnavailable {
		t.Fatal("closed read accepted")
	}
}

func TestCorruptOversizedAndLinkedFilesFailClosed(t *testing.T) {
	for _, mode := range []string{"corrupt", "oversized", "directory", "symlink", "hardlink"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateDirectory(t)
			s := openStore(t, dir, false)
			path := filepath.Join(dir, latestName)
			switch mode {
			case "corrupt":
				if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), remoteprojection.MaxBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "symlink", "hardlink":
				target := filepath.Join(dir, "fixture-target")
				if err := os.WriteFile(target, []byte("synthetic"), 0o600); err != nil {
					t.Fatal(err)
				}
				var err error
				if mode == "symlink" {
					err = os.Symlink("fixture-target", path)
				} else {
					err = os.Link(target, path)
				}
				if err != nil {
					t.Skip("fixture link unavailable on this host")
				}
			}
			if data, err := s.Read(serverID); data != nil || err == nil {
				t.Fatal("unsafe slot accepted")
			}
		})
	}
}

func TestStagingRecoveryAndFailurePreservePrevious(t *testing.T) {
	for _, failure := range []string{"sync", "replace", "short-write", "disk-full"} {
		t.Run(failure, func(t *testing.T) {
			dir := privateDirectory(t)
			s := openStore(t, dir, true)
			raw, identity := sample()
			if err := s.Publish(raw, identity); err != nil {
				t.Fatal(err)
			}
			previous, err := s.Read(serverID)
			if err != nil {
				t.Fatal(err)
			}
			originalSync, originalReplace := s.syncFile, s.replace
			originalWrite := s.writeFile
			switch failure {
			case "sync":
				s.syncFile = func(*os.File) error { return errors.New("synthetic failure") }
			case "replace":
				s.replace = func(string, string) error { return errors.New("synthetic failure") }
			case "short-write", "disk-full":
				s.writeFile = func(f *os.File, data []byte) (int, error) {
					n, err := f.Write(data[:len(data)/2])
					if failure == "disk-full" {
						return n, errors.New("synthetic disk full")
					}
					return n, err
				}
			}
			raw.ObservedAt = raw.ObservedAt.Add(time.Minute)
			if err := s.Publish(raw, identity); !errors.Is(err, ErrUnavailable) {
				t.Fatal("injected failure not reported")
			}
			got, err := s.Read(serverID)
			if err != nil || !bytes.Equal(got, previous) {
				t.Fatal("pre-replacement failure damaged previous snapshot")
			}
			s.syncFile, s.replace = originalSync, originalReplace
			s.writeFile = originalWrite
			if err := s.Publish(raw, identity); err != nil {
				t.Fatal(err)
			}
			got, err = s.Read(serverID)
			if err != nil || bytes.Equal(got, previous) {
				t.Fatal("staging recovery did not publish")
			}
			if _, err := os.Stat(filepath.Join(dir, stagingName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("staging retained after success")
			}
		})
	}
}

func TestConcurrentReaderNeverReturnsPartialDocument(t *testing.T) {
	dir := privateDirectory(t)
	w := openStore(t, dir, true)
	r := openStore(t, dir, false)
	r.checkFile = func(f *os.File) error {
		err := privateHandle(f, false)
		if err != nil {
			t.Log("synthetic handle validation:", err)
		}
		return err
	}
	raw, identity := sample()
	if err := w.Publish(raw, identity); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			data, err := r.Read(serverID)
			if err != nil && !errors.Is(err, ErrUnavailable) {
				t.Errorf("unexpected concurrent read failure: %v", err)
			}
			if err == nil && remoteprojection.Validate(data, serverID) != nil {
				t.Error("partial document returned")
			}
		}
	}()
	for i := 0; i < 20; i++ {
		raw.ObservedAt = raw.ObservedAt.Add(time.Second)
		// Windows may report a sharing violation; a later collection can retry.
		if err := w.Publish(raw, identity); err != nil && !errors.Is(err, ErrUnavailable) {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if data, err := r.Read(serverID); err != nil || remoteprojection.Validate(data, serverID) != nil {
		t.Fatal("no valid document after concurrent publication")
	}
}

func TestReplacementDuringHandleValidationIsUnavailable(t *testing.T) {
	dir := privateDirectory(t)
	w := openStore(t, dir, true)
	r := openStore(t, dir, false)
	raw, id := sample()
	if err := w.Publish(raw, id); err != nil {
		t.Fatal(err)
	}
	r.checkFile = func(f *os.File) error {
		raw.ObservedAt = raw.ObservedAt.Add(time.Second)
		if err := w.Publish(raw, id); err != nil {
			t.Fatal(err)
		}
		// Force the privacy-failure path after the opened target was replaced.
		return ErrUnsafe
	}
	if data, err := r.Read(serverID); data != nil || err != ErrUnavailable {
		t.Fatal("replacement during validation was not classified as unavailable")
	}
}

func TestUnavailableHandleNeverReturnsBytes(t *testing.T) {
	s := openStore(t, privateDirectory(t), true)
	raw, id := sample()
	if err := s.Publish(raw, id); err != nil {
		t.Fatal(err)
	}
	s.checkFile = func(*os.File) error { return ErrUnavailable }
	if data, err := s.Read(serverID); data != nil || err != ErrUnavailable {
		t.Fatal("unavailable handle became readable or a permission error")
	}
}

func TestInvalidProjectionDoesNotTouchSlot(t *testing.T) {
	dir := privateDirectory(t)
	s := openStore(t, dir, true)
	raw, id := sample()
	id.SourceID = "bad"
	if s.Publish(raw, id) != ErrInvalid {
		t.Fatal("invalid projection accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != lockName {
		t.Fatal("invalid input touched slot")
	}
}

func TestDirectorySyncFailureReportsFailureWithWholeNewDocument(t *testing.T) {
	s := openStore(t, privateDirectory(t), true)
	raw, id := sample()
	s.syncDir = func(*os.File) error { return errors.New("synthetic directory sync failure") }
	if err := s.Publish(raw, id); err != ErrUnavailable {
		t.Fatal("directory sync failure hidden")
	}
	got, err := s.Read(serverID)
	want, _ := remoteprojection.Encode(raw, id)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("replacement is not a complete document")
	}
}

func TestUnsafeExistingArtifactsAreNotRepaired(t *testing.T) {
	for _, name := range []string{lockName, stagingName, latestName} {
		t.Run(name, func(t *testing.T) {
			dir := privateDirectory(t)
			path := filepath.Join(dir, name)
			original := bytes.Repeat([]byte("x"), remoteprojection.MaxBytes+1)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(dir, true)
			if name == lockName {
				if err != ErrUnsafe {
					if s != nil {
						s.Close()
					}
					t.Fatal("unsafe lock accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				raw, id := sample()
				if err := s.Publish(raw, id); err != ErrUnsafe {
					t.Fatal("unsafe target accepted")
				}
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatal("existing artifact modified")
			}
		})
	}
}

func TestOpenDoesNotCreateMissingDirectory(t *testing.T) {
	path := filepath.Join(privateDirectory(t), "missing")
	if s, err := Open(path, true); err != ErrUnsafe {
		if s != nil {
			s.Close()
		}
		t.Fatal("missing directory accepted")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing directory created")
	}
}
