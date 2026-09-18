//go:build linux

package connectedinstall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

func TestLedgerIdentityAtAnchoredPrivateState(t *testing.T) {
	if os.Geteuid() != 0 || os.Getuid() != 0 {
		t.Skip("owned root fixture")
	}
	t.Setenv("TMPDIR", "/var/lib")
	state := t.TempDir()
	const uid, gid = 65001, 65002
	ledger := filepath.Join(state, "ledger")
	if err := os.Mkdir(ledger, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(ledger, uid, gid); err != nil {
		t.Fatal(err)
	}
	member := func(name string, data []byte) {
		t.Helper()
		path := filepath.Join(ledger, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	member("upload.sqlite", []byte("synthetic database identity"))
	member(".upload-lock", nil)
	ctx := context.Background()
	before, err := ledgerIdentityAt(ctx, state, uid, gid)
	if err == ledgeridentity.ErrUnavailable && os.Getenv("OBSERVER_EXT4_ACCEPTANCE") != "1" {
		t.Skip("ext4 ioctl unavailable")
	}
	if err != nil || ledgeridentity.Validate(before) != nil {
		t.Fatal("anchored fixture refused")
	}
	again, err := ledgerIdentityAt(ctx, state, uid, gid)
	if err != nil || again != before {
		t.Fatal("stable private ledger identity changed")
	}
	member("replacement", []byte("replacement database"))
	if err := os.Rename(filepath.Join(ledger, "replacement"), filepath.Join(ledger, "upload.sqlite")); err != nil {
		t.Fatal(err)
	}
	after, err := ledgerIdentityAt(ctx, state, uid, gid)
	if err != nil || after.Database == before.Database {
		t.Fatal("database replacement was not detected")
	}
	if err := os.Symlink("upload.sqlite", filepath.Join(ledger, "foreign")); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerIdentityAt(ctx, state, uid, gid); err == nil {
		t.Fatal("unknown member accepted")
	}
}
