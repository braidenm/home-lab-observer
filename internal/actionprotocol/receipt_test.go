package actionprotocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func receiptVector(t *testing.T) ([]byte, string) {
	t.Helper()
	raw, err := os.ReadFile("testdata/paired-container-execution-v1.json")
	if err != nil {
		t.Fatal("paired fixture missing")
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "7dec7af8fe21301534442d2ef55654c70dde83fda267ad7866c20df21d65612e" {
		t.Fatal("paired fixture source drift")
	}
	var fixture struct {
		CanonicalReceipt struct {
			Preimage string `json:"preimage"`
			SHA256   string `json:"sha256"`
		} `json:"canonical_receipt"`
	}
	if json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture encoding")
	}
	return []byte(strings.TrimPrefix(fixture.CanonicalReceipt.Preimage, "platform-container-receipt/v1\n")), fixture.CanonicalReceipt.SHA256
}

func TestPairedReceiptCanonicalDigestMatchesIndependentGolden(t *testing.T) {
	raw, expected := receiptVector(t)
	r, err := DecodeRequest(raw)
	if err != nil {
		t.Fatal("canonical request refused")
	}
	canonical, err := CanonicalRequest(r)
	if err != nil || string(canonical) != string(raw) {
		t.Fatal("sixteen-field canonical order changed")
	}
	digest, err := ReceiptDigest(r)
	if err != nil || hex.EncodeToString(digest[:]) != expected || expected != "be38397e16ee8cd148c05cc8c9ec0adc84652a69ca11a2c41009d3b37d0b8898" {
		t.Fatal("remote identity preimage changed")
	}
	// Accepted input member order is not itself the canonical identity order.
	var unordered map[string]json.RawMessage
	json.Unmarshal(raw, &unordered)
	reordered, _ := json.Marshal(unordered)
	other, err := DecodeRequest(reordered)
	if err != nil || other != r {
		t.Fatal("legal input member order rejected")
	}
}

func TestPairedReceiptRejectsClosedAndUnrepresentableInputs(t *testing.T) {
	raw, _ := receiptVector(t)
	for _, candidate := range [][]byte{
		[]byte(strings.Replace(string(raw), `"policy_revision":"9223372036854775807"`, `"policy_revision":9223372036854775807`, 1)),
		[]byte(strings.Replace(string(raw), `"policy_revision":"9223372036854775807"`, `"policy_revision":"9223372036854775808"`, 1)),
		[]byte(strings.Replace(string(raw), `"policy_revision":"9223372036854775807"`, `"policy_revision":"01"`, 1)),
		[]byte(strings.Replace(string(raw), `"action_id":`, `"Action_id":`, 1)),
		[]byte(strings.Replace(string(raw), `"action_id":`, `"unknown-secret-canary":0,"action_id":`, 1)),
		[]byte(strings.Replace(string(raw), `"action_id":`, `"action_id":"hla_00000000000000000000000000000001","action_id":`, 1)),
		[]byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":1,"format_version":1`, 1)),
		[]byte(strings.Replace(string(raw), `"format_version":1`, `"format_version":1.0`, 1)),
		[]byte(strings.Replace(string(raw), `"actor_user_id":"usr_00000000000000000000000000000001"`, `"actor_user_id":"usr_00000000000000000000000000000002"`, 1)),
		[]byte(strings.Replace(string(raw), "23:55:00.999999Z", "23:54:59.999999Z", 1)),
		[]byte(strings.Replace(string(raw), "23:50:00.999999Z", "23:50:00.999999001Z", 1)),
		append(append([]byte(nil), raw...), []byte(`{}`)...),
		[]byte(strings.Repeat(" ", 16385)),
	} {
		if _, err := DecodeRequest(candidate); err == nil {
			t.Fatal("invalid paired request accepted")
		}
	}
}
