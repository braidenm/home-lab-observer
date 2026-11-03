package actionprotocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotCodecsMatchPairedCanonicalDigest(t *testing.T) {
	p, err := DecodePublication(codecVector(t, "Publication"))
	if err != nil || len(p.Entries) != 32 {
		t.Fatal("complete publication")
	}
	r, err := DecodeSnapshotReceipt(codecVector(t, "SnapshotReceipt"))
	if err != nil {
		t.Fatal("snapshot receipt")
	}
	// The fixture file is pretty-printed. Bounds apply to the actual transmitted
	// bytes; use its compact maximum vector, never raise the production limit.
	var viewRaw bytes.Buffer
	if json.Compact(&viewRaw, codecVector(t, "SnapshotView")) != nil {
		t.Fatal("compact vector")
	}
	v, err := DecodeSnapshotView(viewRaw.Bytes())
	if err != nil || v.Readiness != "READY" || len(v.Entries) != 32 {
		t.Fatal("snapshot view")
	}
	digest, err := SnapshotDigest(v.ServerID, v.OwnerUserID, v.ConnectorInstanceID, r, p.Entries)
	if err != nil || digest != r.SnapshotDigest {
		t.Fatal("canonical snapshot digest differs from paired vector")
	}
}

func TestSnapshotPublicationRefusesOrderingAliasesAndClosedFields(t *testing.T) {
	raw := codecVector(t, "Publication")
	for _, input := range [][]byte{
		[]byte(strings.Replace(string(raw), `"publication_id":`, `"Publication_id":`, 1)),
		[]byte(strings.Replace(string(raw), `"expected_snapshot_revision": "1"`, `"expected_snapshot_revision": 1`, 1)),
		[]byte(strings.Replace(string(raw), `"expected_snapshot_revision": "1"`, `"expected_snapshot_revision": "9223372036854775808"`, 1)),
		[]byte(strings.Replace(string(raw), `"format_version": 1`, `"format_version": 1,"unknown":true`, 1)),
	} {
		if _, err := DecodePublication(input); err == nil {
			t.Fatal("open or unrepresentable publication accepted")
		}
	}
	p, _ := DecodePublication(raw)
	p.Entries[0], p.Entries[1] = p.Entries[1], p.Entries[0]
	if ValidateEntries(p.Entries) == nil {
		t.Fatal("publication was silently sorted")
	}
	p, _ = DecodePublication(raw)
	p.Entries[1].TargetKey = p.Entries[0].TargetKey
	if ValidateEntries(p.Entries) == nil {
		t.Fatal("display alias collision accepted")
	}
	p, _ = DecodePublication(raw)
	p.Entries = append(p.Entries, p.Entries[0])
	if ValidateEntries(p.Entries) == nil {
		t.Fatal("entry cap bypassed")
	}
}

func TestSnapshotMissingAndExpiredViewsDoNotInventAFirstHead(t *testing.T) {
	raw := codecVector(t, "SnapshotView")
	var fields map[string]any
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("fixture")
	}
	fields["readiness"], fields["entries"] = "UNAVAILABLE", []any{}
	encoded, _ := json.Marshal(fields)
	v, err := DecodeSnapshotView(encoded)
	if err != nil || v.SnapshotRevision == nil {
		t.Fatal("expired retained head was lost")
	}
	for _, field := range []string{"snapshot_revision", "snapshot_digest", "advertised_at", "expires_at"} {
		fields[field] = nil
	}
	encoded, _ = json.Marshal(fields)
	v, err = DecodeSnapshotView(encoded)
	if err != nil || v.SnapshotRevision != nil {
		t.Fatal("verified missing view")
	}
	fields["snapshot_revision"] = "1"
	encoded, _ = json.Marshal(fields)
	if _, err := DecodeSnapshotView(encoded); err == nil {
		t.Fatal("partial head interpreted as first publication")
	}
	fields["snapshot_revision"], fields["entries"] = nil, nil
	encoded, _ = json.Marshal(fields)
	if _, err := DecodeSnapshotView(encoded); err == nil {
		t.Fatal("null entries accepted as empty complete set")
	}
}
