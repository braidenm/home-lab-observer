//go:build linux

package connectedinstall

import (
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

func TestActionLayoutWritableLeavesDoNotOwnAncestors(t *testing.T) {
	i := actionprofile.Identity{WorkerUID: 201, WorkerGID: 202, BrokerUID: 203, BrokerGID: 204, IPCGID: 205}
	expected := map[string]directorySpec{
		actionStateRoot + "/worker":  {uid: i.WorkerUID, gid: i.WorkerGID, mode: 0700},
		actionStateRoot + "/journal": {uid: i.BrokerUID, gid: i.BrokerGID, mode: 0700},
		actionRunRoot + "/worker":    {uid: i.BrokerUID, gid: i.IPCGID, mode: 0750},
	}
	seen := make(map[string]bool)
	for _, spec := range actionDirectories(i) {
		path := filepath.Join(spec.parent, spec.name)
		if seen[path] {
			t.Fatalf("duplicate action directory: %s", path)
		}
		seen[path] = true
		want, private := expected[path]
		if private {
			if spec.uid != want.uid || spec.gid != want.gid || spec.mode != want.mode {
				t.Fatalf("wrong role for %s", path)
			}
			delete(expected, path)
		} else if spec.uid != 0 || spec.mode&0022 != 0 {
			t.Fatalf("untrusted ancestor: %s", path)
		}
	}
	if len(expected) != 0 {
		t.Fatal("missing role directories")
	}
}
