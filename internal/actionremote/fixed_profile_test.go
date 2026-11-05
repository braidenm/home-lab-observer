package actionremote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func TestFixedProfileChangesNeverPublish(t *testing.T) {
	for _, change := range []string{"target", "alias", "operation", "grace", "policy", "capability", "count", "addition"} {
		t.Run(change, func(t *testing.T) {
			binding, query, original := controlFixture(t)
			query.Entries = append([]actionprotocol.Entry(nil), query.Entries...)
			switch change {
			case "target":
				// Change the final target without changing canonical ordering.
				last := query.Entries[len(query.Entries)-1].TargetID
				for i := range query.Entries {
					if query.Entries[i].TargetID == last {
						query.Entries[i].TargetID = "hlt_" + strings.Repeat("f", 32)
					}
				}
			case "alias":
				last := query.Entries[len(query.Entries)-1].TargetID
				for i := range query.Entries {
					if query.Entries[i].TargetID == last {
						query.Entries[i].TargetKey = "ctr_" + strings.Repeat("f", 16)
					}
				}
			case "operation":
				// Removing one independently approved operation preserves a valid set.
				query.Entries = query.Entries[1:]
			case "grace":
				for i := range query.Entries {
					if !query.Entries[i].Parameters.StopGraceSeconds.Null {
						value := uint8(1)
						if query.Entries[i].Parameters.StopGraceSeconds.Seconds == 1 {
							value = 2
						}
						query.Entries[i].Parameters.StopGraceSeconds = actionprotocol.Grace{Seconds: value}
						query.Entries[i].ParameterSHA256, _ = actionprotocol.ParameterDigest(query.Entries[i].Operation, query.Entries[i].Parameters)
						break
					}
				}
			case "policy":
				query.Entries[0].PolicyRevision = "2"
			case "capability":
				query.Entries[0].CapabilityRevision = "2"
			case "count":
				query.Entries = []actionprotocol.Entry{}
			case "addition":
				original.SnapshotDigest, _ = actionprotocol.SnapshotDigest(binding.ServerID, binding.OwnerUserID, binding.ConnectorInstanceID, original, query.Entries[:len(query.Entries)-1])
			}
			if actionprotocol.ValidateSnapshotQuery(query) != nil {
				t.Fatal("mutation must be a valid different public profile")
			}
			var posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: binding.ServerID, OwnerUserID: binding.OwnerUserID,
						ConnectorInstanceID: binding.ConnectorInstanceID, Readiness: "UNAVAILABLE", SnapshotRevision: &original.SnapshotRevision,
						SnapshotDigest: &original.SnapshotDigest, AdvertisedAt: &original.AdvertisedAt, ExpiresAt: &original.ExpiresAt, Entries: []actionprotocol.EntryView{}})
					return
				}
				posts.Add(1)
				w.WriteHeader(http.StatusConflict)
			}))
			defer server.Close()
			reply, err := ownedClient(t, server, binding, &credentialsFixture{}).RefreshSnapshot(context.Background(), query)
			if err == nil || reply.Receipt != nil || posts.Load() != 0 {
				t.Fatal("changed first-profile entries reached publication")
			}
		})
	}
}

func TestFixedProfileUnchangedHeadsRefreshWithExactCAS(t *testing.T) {
	for _, readiness := range []string{"READY", "EMPTY", "UNAVAILABLE", "cooldown-only"} {
		t.Run(readiness, func(t *testing.T) {
			binding, query, original := controlFixture(t)
			if readiness == "EMPTY" {
				query.Entries = []actionprotocol.Entry{}
			}
			original.SnapshotDigest, _ = actionprotocol.SnapshotDigest(binding.ServerID, binding.OwnerUserID, binding.ConnectorInstanceID, original, query.Entries)
			head := actionprotocol.SnapshotView{ServerID: binding.ServerID, OwnerUserID: binding.OwnerUserID, ConnectorInstanceID: binding.ConnectorInstanceID,
				Readiness: readiness, SnapshotRevision: &original.SnapshotRevision, SnapshotDigest: &original.SnapshotDigest,
				AdvertisedAt: &original.AdvertisedAt, ExpiresAt: &original.ExpiresAt, Entries: []actionprotocol.EntryView{}}
			if readiness != "UNAVAILABLE" {
				for _, entry := range query.Entries {
					head.Entries = append(head.Entries, actionprotocol.EntryView{Entry: entry})
				}
			}
			if readiness == "cooldown-only" {
				head.Readiness = "READY"
				head.Entries[0].CooldownUntil = &original.ExpiresAt
			}
			next := original
			next.SnapshotRevision = "2"
			next.SnapshotDigest, _ = actionprotocol.SnapshotDigest(binding.ServerID, binding.OwnerUserID, binding.ConnectorInstanceID, next, query.Entries)
			var gets, posts atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					gets.Add(1)
					json.NewEncoder(w).Encode(head)
					return
				}
				posts.Add(1)
				var publication actionprotocol.Publication
				if json.NewDecoder(r.Body).Decode(&publication) != nil || publication.ExpectedSnapshotRevision == nil || *publication.ExpectedSnapshotRevision != original.SnapshotRevision {
					t.Error("unchanged head lost its CAS revision")
				}
				json.NewEncoder(w).Encode(next)
			}))
			defer server.Close()
			reply, err := ownedClient(t, server, binding, &credentialsFixture{}).RefreshSnapshot(context.Background(), query)
			if err != nil || actionprotocol.MatchSnapshotReply(query, reply) != nil || gets.Load() != 1 || posts.Load() != 1 {
				t.Fatal("unchanged fixed profile could not refresh once")
			}
		})
	}
}

func TestFixedProfileCASConflictNeverRetries(t *testing.T) {
	binding, query, original := controlFixture(t)
	var gets, posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			gets.Add(1)
			json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: binding.ServerID, OwnerUserID: binding.OwnerUserID, ConnectorInstanceID: binding.ConnectorInstanceID,
				Readiness: "UNAVAILABLE", SnapshotRevision: &original.SnapshotRevision, SnapshotDigest: &original.SnapshotDigest,
				AdvertisedAt: &original.AdvertisedAt, ExpiresAt: &original.ExpiresAt, Entries: []actionprotocol.EntryView{}})
			return
		}
		posts.Add(1)
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	reply, err := ownedClient(t, server, binding, &credentialsFixture{}).RefreshSnapshot(context.Background(), query)
	if err == nil || reply.Receipt != nil || gets.Load() != 1 || posts.Load() != 1 {
		t.Fatal("concurrent CAS conflict retried or became success")
	}
}
