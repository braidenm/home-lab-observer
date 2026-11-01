//go:build !linux

package actionengine

import (
	"context"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestUnsupportedPlatformHasNoActionAdapter(t *testing.T) {
	if a, err := Open(context.Background(), SocketWitness{Device: 1, Inode: 1, GroupID: 1}); a != nil || err != ErrUnsupported {
		t.Fatal("unsupported platform adapter")
	}
	var a Engine
	state, err := a.Inspect(context.Background(), "untrusted")
	if state != actioncore.UnknownState || err != ErrUnsupported || a.Start(context.Background(), "untrusted") != ErrUnsupported || a.Stop(context.Background(), "untrusted", 1) != ErrUnsupported || a.Restart(context.Background(), "untrusted", 1) != ErrUnsupported || a.Close() != nil {
		t.Fatal("unsupported platform operation")
	}
}
