package actionprotocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLocalNoClaimIsExactBodylessExecutionContinuation(t *testing.T) {
	raw := []byte(`{"version":"observer-action-local/v1","kind":"NO_CLAIM","conversationNonce":"00000000000000000000000000000001","ordinal":"2","payload":{}}`)
	hash := sha256.Sum256(raw)
	if len(raw) != 138 || hex.EncodeToString(hash[:]) != "ef6c131059feece6eac3fca69b3ade1a9401508e259ececea45749a9b7fe703f" {
		t.Fatal("independent no-claim golden drift")
	}
	if _, err := DecodeEnvelope(raw); err != nil {
		t.Fatal("reviewed no-claim kind refused")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), WorkerVersion, AdminVersion, 1),
		strings.Replace(string(raw), `"ordinal":"2"`, `"ordinal":"1"`, 1),
		strings.Replace(string(raw), `"payload":{}`, `"payload":{"claimed":false}`, 1),
		strings.Replace(string(raw), `00000000000000000000000000000001`, `00000000000000000000000000000000`, 1),
	} {
		if _, err := DecodeEnvelope([]byte(bad)); err == nil {
			t.Fatal("no-claim grammar broadened")
		}
	}
}

func TestLocalEnvelopeCanonicalPairedKinds(t *testing.T) {
	receiptVector(t)
	raw, _ := os.ReadFile("testdata/paired-container-execution-v1.json")
	var fixture struct {
		Worker  map[string]json.RawMessage `json:"positive_worker_ipc"`
		Admin   map[string]json.RawMessage `json:"positive_admin_ipc"`
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture")
	}
	for _, kinds := range []map[string]json.RawMessage{fixture.Worker, fixture.Admin} {
		for kind, raw := range kinds {
			t.Run(kind, func(t *testing.T) {
				if kind == "RECOVERY_STATE" {
					var maximum bytes.Buffer
					json.Compact(&maximum, raw)
					if _, err := DecodeEnvelope(maximum.Bytes()); err == nil {
						t.Fatal("MAX sizing release accepted")
					}
					var template Envelope
					json.Unmarshal(raw, &template)
					var state RecoveryState
					json.Unmarshal(template.Payload, &state)
					view := reachableRecoveryView(t, fixture.Objects["RecoveryView"])
					json.Unmarshal(view, &state.Remote)
					template.Payload, _ = json.Marshal(state)
					raw, _ = json.Marshal(template)
				}
				if kind == "RECOVERY_QUERY" {
					// This size template deliberately uses LongMAX, unreachable for
					// the eight-approval profile. Preserve the fixture; use its valid
					// typed CHECK body for the consumed positive state instead.
					var maximum bytes.Buffer
					json.Compact(&maximum, raw)
					if _, err := DecodeEnvelope(maximum.Bytes()); err == nil {
						t.Fatal("unreachable approval history accepted")
					}
					var template struct {
						Version string        `json:"version"`
						Kind    string        `json:"kind"`
						Nonce   string        `json:"conversationNonce"`
						Ordinal string        `json:"ordinal"`
						Payload RecoveryQuery `json:"payload"`
					}
					json.Unmarshal(raw, &template)
					template.Payload.Body = fixture.Objects["RecoveryCheck"]
					raw, _ = json.Marshal(template)
				}
				var compact bytes.Buffer
				json.Compact(&compact, raw)
				envelope, err := DecodeEnvelope(compact.Bytes())
				if err != nil {
					t.Fatal("canonical paired kind refused")
				}
				encoded, err := EncodeEnvelope(envelope)
				if err != nil || !bytes.Equal(encoded, compact.Bytes()) {
					t.Fatal("canonical paired bytes changed")
				}
				for _, bad := range []string{
					strings.Replace(compact.String(), `"kind":`, `"Kind":`, 1),
					strings.Replace(compact.String(), `"ordinal":"`, `"ordinal":"0`, 1),
					strings.Replace(compact.String(), `"payload":`, `"payload":{},"payload":`, 1),
					compact.String() + "\n",
				} {
					if _, err := DecodeEnvelope([]byte(bad)); err == nil {
						t.Fatal("noncanonical envelope accepted")
					}
				}
			})
		}
	}
}

func TestLocalSnapshotEnvelopeUsesExactControlVectors(t *testing.T) {
	query, reply := startupFixture(t)
	for _, test := range []struct {
		kind, ordinal string
		body          any
	}{{"SNAPSHOT_QUERY", "1", query}, {"SNAPSHOT_REPLY", "2", reply}} {
		payload, _ := json.Marshal(test.body)
		raw, err := EncodeEnvelope(Envelope{Version: WorkerVersion, Kind: test.kind, ConversationNonce: query.PublicationID, Ordinal: test.ordinal, Payload: payload})
		if err != nil {
			t.Fatal("snapshot control envelope")
		}
		if _, err = DecodeEnvelope(raw); err != nil {
			t.Fatal("control roundtrip")
		}
	}
}
