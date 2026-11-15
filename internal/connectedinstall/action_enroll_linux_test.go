//go:build linux

package connectedinstall

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

func TestActionGrantPipeIsPrivateAndEndsAtGrant(t *testing.T) {
	i := actionprofile.Identity{WorkerUID: uint32(os.Geteuid()), WorkerGID: uint32(os.Getegid())}
	if i.WorkerUID == 0 || i.WorkerGID == 0 {
		// Root can assign an ordinary synthetic worker without adding accounts.
		i.WorkerUID, i.WorkerGID = 65534, 65534
	}
	raw := []byte("hlae_" + strings.Repeat("1", 32) + "." + strings.Repeat("A", 43))
	f, err := actionGrantPipe(raw, i)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeNamedPipe == 0 || info.Mode().Perm() != 0600 ||
		st.Uid != i.WorkerUID || st.Gid != i.WorkerGID {
		t.Fatal("grant pipe is not privately owned by the worker")
	}
	got, err := io.ReadAll(io.LimitReader(f, 82))
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("grant pipe did not carry exactly one bounded grant")
	}
	if f, err := actionGrantPipe(raw[:80], i); err == nil {
		f.Close()
		t.Fatal("short grant reached a pipe")
	}
}
