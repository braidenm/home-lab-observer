package actionprotocol

import "testing"

func TestFixedProfilePartialHeadsRefuseWithoutPanic(t *testing.T) {
	query, reply := startupFixture(t)
	for _, missing := range []string{"revision", "digest", "advertised", "expiry"} {
		t.Run(missing, func(t *testing.T) {
			r := *reply.Receipt
			head := SnapshotView{ServerID: query.ServerID, OwnerUserID: query.OwnerUserID, ConnectorInstanceID: query.ConnectorInstanceID,
				Readiness: "UNAVAILABLE", SnapshotRevision: &r.SnapshotRevision, SnapshotDigest: &r.SnapshotDigest,
				AdvertisedAt: &r.AdvertisedAt, ExpiresAt: &r.ExpiresAt, Entries: []EntryView{}}
			switch missing {
			case "revision":
				head.SnapshotRevision = nil
			case "digest":
				head.SnapshotDigest = nil
			case "advertised":
				head.AdvertisedAt = nil
			case "expiry":
				head.ExpiresAt = nil
			}
			if MatchSnapshotHead(query, head) == nil {
				t.Fatal("partial head became initial publication")
			}
		})
	}
}
