package actioncore

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestJournalMetadataPreservesFingerprintAndCanonicalRoundTrip(t *testing.T) {
	f := newFixture()
	// Independently frozen original version-1 preimage, before the metadata refactor.
	if fmt.Sprintf("%x", fingerprint(f.request, f.binding)) != "2fba09c56dcd43aafbce3b72362d701623e2e05e0b73d46fd480c51484b7d08d" {
		t.Fatal("original fingerprint golden changed")
	}
	f.request.PolicyRevision = math.MaxUint64
	f.request.CapabilityRevision = math.MaxUint64
	f.binding.PolicyRevision = f.request.PolicyRevision
	f.binding.CapabilityRevision = f.request.CapabilityRevision
	key, err := NewJournalKey(f.request, f.binding)
	if err != nil {
		t.Fatal("metadata construction")
	}
	raw, err := CanonicalJournalMetadata(key)
	if err != nil || len(raw) > 2048 || sha256.Sum256(raw) != fingerprint(f.request, f.binding) {
		t.Fatal("version one fingerprint changed")
	}
	parsed, err := ParseJournalMetadata(raw)
	if err != nil || parsed.Fingerprint != key.Fingerprint || parsed.Metadata.Request.PolicyRevision != math.MaxUint64 {
		t.Fatal("canonical roundtrip")
	}
	if raw2, _ := CanonicalJournalMetadata(parsed); !bytes.Equal(raw, raw2) {
		t.Fatal("UTC bytes changed")
	}
	f.delivery.LeaseDeadline = f.delivery.LeaseDeadline.Add(time.Hour)
	again, _ := NewJournalKey(f.request, f.binding)
	if again.Fingerprint != key.Fingerprint {
		t.Fatal("lease became identity")
	}
	for _, bad := range [][]byte{append([]byte(" "), raw...), append(raw, []byte(" {}")...), bytes.Replace(raw, []byte(`"Version":1`), []byte(`"Version":2`), 1), bytes.Replace(raw, []byte(`"Version":1`), []byte(`"Unknown":1,"Version":1`), 1)} {
		if _, err := ParseJournalMetadata(bad); err == nil {
			t.Fatal("noncanonical metadata accepted")
		}
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"Version":1`), []byte(`"Version":1,"Version":1`), 1), bytes.Replace(raw, []byte("2026-11-01T12:00:00Z"), []byte("2026-11-01T12:00:00+00:00"), 1)} {
		if _, err := ParseJournalMetadata(bad); err == nil {
			t.Fatal("duplicate/noncanonical UTC accepted")
		}
	}
	key.Fingerprint[0] ^= 1
	if _, err := CanonicalJournalMetadata(key); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}
