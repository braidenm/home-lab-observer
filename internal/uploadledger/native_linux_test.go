//go:build linux

package uploadledger

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

func TestBothPendingSchemasSurviveSQLiteReopen(t *testing.T) {
	for _, native := range []bool{false, true} {
		l, dir := provision(t)
		initial := load(t, l)
		next := pending(t, 7, 0)
		if native {
			raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at}
			raw.CPU.State, raw.Memory.State, raw.Uptime.State, raw.Filesystems.State = observation.Unknown, observation.Unknown, observation.Unknown, observation.Unknown
			body, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: binding.ServerID, Version: "0.1.0", OS: "linux"})
			if err != nil {
				t.Fatal(err)
			}
			next.Pending.Body, next.Pending.Digest = body, sha256.Sum256(body)
		}
		if err := l.Commit(context.Background(), initial, next); err != nil {
			t.Fatal(err)
		}
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenExisting(context.Background(), dir, binding)
		if err != nil {
			t.Fatal(err)
		}
		got := load(t, reopened)
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
		if !sameRecord(got, next) {
			t.Fatal("persisted pending bytes, digest, binding or sequence changed")
		}
	}
}
