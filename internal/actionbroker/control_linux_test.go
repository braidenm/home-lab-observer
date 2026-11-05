//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func controlReply(t *testing.T, q actionprotocol.SnapshotQuery) actionprotocol.SnapshotReply {
	t.Helper()
	when := time.Now().UTC().Truncate(time.Microsecond)
	r := actionprotocol.SnapshotReceipt{PublicationID: q.PublicationID, SnapshotRevision: "1", SnapshotDigest: strings.Repeat("0", 64), AdvertisedAt: when.Format(actionprotocol.HTTPTimeLayout), ExpiresAt: when.Add(60 * time.Second).Format(actionprotocol.HTTPTimeLayout)}
	var err error
	r.SnapshotDigest, err = actionprotocol.SnapshotDigest(q.ServerID, q.OwnerUserID, q.ConnectorInstanceID, r, q.Entries)
	if err != nil {
		t.Fatal("synthetic authenticated receipt")
	}
	return actionprotocol.SnapshotReply{Outcome: "OK", Receipt: &r}
}

func TestControlInstallsReadinessOnlyAfterExactBoundedReplyAndNewBegin(t *testing.T) {
	b, _ := reservationFixture(t)
	b.readiness = nil
	c, err := b.beginControl(context.Background())
	if err != nil || len(b.live) != 0 {
		t.Fatal("control must not reserve execution capacity")
	}
	if _, err := b.beginControl(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal("second control queued")
	}
	if _, err := b.reserve(context.Background()); err == nil {
		t.Fatal("claim before readiness")
	}
	if b.completeControl(context.Background(), c, controlReply(t, c.query)) != nil {
		t.Fatal("exact fresh reply")
	}
	if b.completeControl(context.Background(), c, controlReply(t, c.query)) == nil {
		t.Fatal("control replay installed readiness")
	}
	r, err := b.reserve(context.Background())
	if err != nil || r.nonce == c.query.PublicationID {
		t.Fatal("new begin lacks independent execution nonce")
	}
	if b.releaseUnclaimed(context.Background(), r) != nil || b.releaseUnclaimed(context.Background(), r) == nil {
		t.Fatal("known NO_CLAIM release/replay")
	}
}

func TestControlCanceledAfterActualNativeSampleCannotInstallReadiness(t *testing.T) {
	b, _ := reservationFixture(t)
	b.readiness = nil
	c, err := b.beginControl(context.Background())
	if err != nil {
		t.Fatal("control")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sampled := false
	b.afterControlSample = func() { sampled = true; cancel() }
	if b.completeControl(ctx, c, controlReply(t, c.query)) == nil || !sampled || b.readiness != nil || b.control != nil {
		t.Fatal("cancellation after native sampling installed readiness")
	}
}

func TestControlPolicyChangeQuiescenceFailureAndEmptyPolicy(t *testing.T) {
	for _, change := range []string{"policy", "quiesce", "digest", "cancel"} {
		t.Run(change, func(t *testing.T) {
			b, _ := reservationFixture(t)
			b.readiness = nil
			c, err := b.beginControl(context.Background())
			if err != nil {
				t.Fatal("control")
			}
			reply := controlReply(t, c.query)
			ctx := context.Background()
			switch change {
			case "policy":
				b.policy.Revision = "2"
			case "quiesce":
				b.quiescing = true
			case "digest":
				reply.Receipt.SnapshotDigest = "0000000000000000000000000000000000000000000000000000000000000000"
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if b.completeControl(ctx, c, reply) == nil || b.readiness != nil {
				t.Fatal("invalid control installed readiness")
			}
		})
	}
	b, _ := reservationFixture(t)
	b.policy.Current = []string{}
	b.policy.Mappings = []mappingDocument{}
	b.readiness = nil
	c, err := b.beginControl(context.Background())
	if err != nil || c.query.Entries == nil || len(c.query.Entries) != 0 {
		t.Fatal("empty policy control must publish exact empty set")
	}
	if b.completeControl(context.Background(), c, controlReply(t, c.query)) != nil {
		t.Fatal("empty publication")
	}
	if _, err := b.reserve(context.Background()); !errors.Is(err, ErrNoEligibleTarget) {
		t.Fatal("empty policy issued execution nonce")
	}
}
