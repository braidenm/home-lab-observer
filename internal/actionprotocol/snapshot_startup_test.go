package actionprotocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotControlCanonicalMaximaMatchIndependentNodeVectors(t *testing.T) {
	query, reply := startupFixture(t)
	// Independently calculated with Node24 from the unchanged hash-bound paired
	// fixture, not adopted dynamically from this Go encoder's output.
	for _, vector := range []struct {
		kind, ordinal string
		payload       any
		size          int
		hash          string
	}{
		{"SNAPSHOT_QUERY", "1", query, 11546, "12cf583d56161bc8c02e94d36865dfd567c206d0a26da3bb4510873ac5147928"},
		{"SNAPSHOT_REPLY", "2", reply, 438, "8a99d5071068c81e9b6b1eecd03a7117ace52ffd0673d0d6d84db8f9cfcb4b7b"},
	} {
		envelope := struct {
			Version           string `json:"version"`
			Kind              string `json:"kind"`
			ConversationNonce string `json:"conversationNonce"`
			Ordinal           string `json:"ordinal"`
			Payload           any    `json:"payload"`
		}{"observer-action-local/v1", vector.kind, query.PublicationID, vector.ordinal, vector.payload}
		raw, err := json.Marshal(envelope)
		digest := sha256.Sum256(raw)
		if err != nil || len(raw) != vector.size || len(raw) > MaxEnvelopeBytes || hex.EncodeToString(digest[:]) != vector.hash {
			t.Fatalf("canonical %s maximum drift", vector.kind)
		}
	}
}

func startupFixture(t *testing.T) (SnapshotQuery, SnapshotReply) {
	t.Helper()
	p, err := DecodePublication(codecVector(t, "Publication"))
	if err != nil {
		t.Fatal("publication fixture")
	}
	r, err := DecodeSnapshotReceipt(codecVector(t, "SnapshotReceipt"))
	if err != nil {
		t.Fatal("receipt fixture")
	}
	request, _ := DecodeRequest(codecVector(t, "RequestReceipt"))
	return SnapshotQuery{request.ServerID, request.OwnerUserID, request.ConnectorInstanceID, r.PublicationID, p.Entries}, SnapshotReply{"OK", &r}
}

func TestSnapshotControlMatchesExactBindingAndReceipt(t *testing.T) {
	q, r := startupFixture(t)
	query, _ := json.Marshal(q)
	decoded, err := DecodeSnapshotQuery(query)
	if err != nil || len(decoded.Entries) != 32 {
		t.Fatal("complete control query")
	}
	reply, _ := json.Marshal(r)
	parsed, err := DecodeSnapshotReply(reply)
	if err != nil || MatchSnapshotReply(decoded, parsed) != nil {
		t.Fatal("correlated fresh publication reply")
	}
	changed := decoded
	changed.Entries = append([]Entry(nil), decoded.Entries...)
	changed.Entries[0].PolicyRevision = "1"
	if MatchSnapshotReply(changed, parsed) == nil {
		t.Fatal("changed approval accepted original receipt")
	}
	changed = decoded
	changed.OwnerUserID = "usr_" + strings.Repeat("f", 32)
	if MatchSnapshotReply(changed, parsed) == nil {
		t.Fatal("publication moved to another owner")
	}
	changed = decoded
	changed.PublicationID = strings.Repeat("f", 32)
	if MatchSnapshotReply(changed, parsed) == nil {
		t.Fatal("old publication ID accepted as fresh")
	}
}

func TestSnapshotControlRefusesOpenPayloadAndFailureReceipt(t *testing.T) {
	q, r := startupFixture(t)
	query, _ := json.Marshal(q)
	for _, raw := range [][]byte{
		bytes.Replace(query, []byte(`"server_id":`), []byte(`"Server_id":`), 1),
		bytes.Replace(query, []byte(`"publication_id":`), []byte(`"extra":true,"publication_id":`), 1),
		bytes.Replace(query, []byte(`"publication_id":"`+q.PublicationID+`"`), []byte(`"publication_id":"`+strings.Repeat("0", 32)+`"`), 1),
	} {
		if _, err := DecodeSnapshotQuery(raw); err == nil {
			t.Fatal("unbound control query")
		}
	}
	for _, outcome := range []string{"DENIED", "UNAVAILABLE", "CONFLICT"} {
		raw, _ := json.Marshal(SnapshotReply{outcome, nil})
		if _, err := DecodeSnapshotReply(raw); err != nil {
			t.Fatal("closed failure unavailable")
		}
		raw, _ = json.Marshal(SnapshotReply{outcome, r.Receipt})
		if _, err := DecodeSnapshotReply(raw); err == nil {
			t.Fatal("failure carried a usable receipt")
		}
	}
	if _, err := DecodeSnapshotReply([]byte(`{"outcome":"OK","receipt":null}`)); err == nil {
		t.Fatal("OK without authenticated receipt")
	}
}
