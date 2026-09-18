package ledgerwitness

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var binding = uploadstate.Binding{ServerID: "srv_0123456789abcdef0123456789abcdef", ConnectorID: "agent_0123456789abcdef0123456789abcdef"}

func pending(t *testing.T, sequence int64, at time.Time) uploadstate.Record {
	t.Helper()
	body, err := remoteprojection.Encode(observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at}, remoteprojection.Identity{SourceID: binding.ServerID, Version: "0.1.0-preview.3", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	return uploadstate.Record{Binding: binding, Watermark: sequence, Pending: &uploadstate.Pending{Sequence: sequence, Body: body, Digest: sha256.Sum256(body), CollectedAt: at}}
}

func TestInitialEncodingGolden(t *testing.T) {
	got, err := Fingerprint(uploadstate.Record{Binding: binding}, binding)
	if err != nil {
		t.Fatal(err)
	}
	// Independently specified length-prefixed v1 wire bytes, not encoder helpers.
	wire := "0000001a6f627365727665722d6c65646765722d7769746e6573732f7631" +
		"000000247372765f3031323334353637383961626364656630313233343536373839616263646566" +
		"000000266167656e745f3031323334353637383961626364656630313233343536373839616263646566" +
		"000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000" + "0000000000"
	raw, err := hex.DecodeString(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != sha256.Sum256(raw) {
		t.Fatal("encoding contract changed")
	}
}

func TestFingerprintDistinguishesLogicalStatesWithoutMutation(t *testing.T) {
	at := time.Date(2001, 1, 2, 3, 4, 5, 1, time.UTC) // Deliberately long expired.
	p := pending(t, 7, at)
	terminal := uploadstate.CloneRecord(p)
	terminal.Stopped = uploadstate.CredentialRejected
	ack := uploadstate.Record{Binding: binding, Watermark: 7, HasAck: true, LastAck: p.Pending.Digest}
	otherAck := ack
	otherAck.LastAck[0] ^= 1
	otherConnector := uploadstate.Record{Binding: binding}
	otherConnector.Binding.ConnectorID = "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherServer := uploadstate.Record{Binding: binding}
	otherServer.Binding.ServerID = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	states := []uploadstate.Record{{Binding: binding}, {Binding: binding, Watermark: 7}, {Binding: binding, Watermark: 7, HasAck: true}, p, terminal, ack, otherAck, pending(t, 8, at), pending(t, 7, at.Add(time.Nanosecond)), otherConnector, otherServer}
	seen := map[[32]byte]bool{}
	for _, r := range states {
		before := uploadstate.CloneRecord(r)
		got, err := Fingerprint(r, r.Binding)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Fingerprint(r, r.Binding)
		if err != nil || again != got || seen[got] {
			t.Fatal("unstable or colliding witness")
		}
		seen[got] = true
		if !reflect.DeepEqual(before, r) {
			t.Fatal("fingerprint mutated input")
		}
	}
	zone := uploadstate.CloneRecord(p)
	zone.Pending.CollectedAt = at.In(time.FixedZone("synthetic", 3600))
	a, _ := Fingerprint(p, binding)
	b, err := Fingerprint(zone, binding)
	if err != nil || a != b {
		t.Fatal("same instant must normalize to UTC")
	}
}

func TestInvalidRecordsReturnNoWitness(t *testing.T) {
	p := pending(t, 1, time.Date(2001, 1, 2, 3, 4, 5, 0, time.UTC))
	for _, mutate := range []func(*uploadstate.Record){
		func(r *uploadstate.Record) { r.Binding.ConnectorID = "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		func(r *uploadstate.Record) { r.Watermark = -1 },
		func(r *uploadstate.Record) { r.Pending.Sequence++ },
		func(r *uploadstate.Record) { r.Pending.Body[0] = '!' },
		func(r *uploadstate.Record) { r.Pending.Digest[0] ^= 1 },
		func(r *uploadstate.Record) { r.Pending.CollectedAt = r.Pending.CollectedAt.Add(time.Second) },
		func(r *uploadstate.Record) { r.LastAck[0] = 1 },
		func(r *uploadstate.Record) { r.Stopped = "unknown" },
	} {
		r := uploadstate.CloneRecord(p)
		mutate(&r)
		got, err := Fingerprint(r, binding)
		if err != ErrUnsafe || got != [32]byte{} {
			t.Fatal("invalid state yielded witness")
		}
	}
}
