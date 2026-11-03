package actionprotocol

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func codecVector(t *testing.T, name string) []byte {
	t.Helper()
	receiptVector(t) // Enforce the full shared fixture's independent source hash.
	raw, _ := os.ReadFile("testdata/paired-container-execution-v1.json")
	var fixture struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.Objects[name] == nil {
		t.Fatal("typed vector absent")
	}
	return fixture.Objects[name]
}

func TestExecutionDTOsConsumePairedVectorsAndExactRequestBinding(t *testing.T) {
	claim, err := DecodeClaim(codecVector(t, "Claimed"))
	if err != nil {
		t.Fatal("valid claim")
	}
	decision, err := DecodeExecutionDecision(codecVector(t, "ExecutionCheckDecision"))
	if err != nil || MatchExecutionDecision(claim, "2", strings.Repeat("0", 31)+"1", decision) != nil {
		t.Fatal("valid correlated checkpoint")
	}
	changed := decision
	changed.Request.CorrelationID = "different-original-correlation"
	if MatchExecutionDecision(claim, "2", decision.CheckNonce, changed) == nil {
		t.Fatal("subset/private-fingerprint equality accepted a different full request")
	}
	changed = decision
	changed.Checkpoint = "1"
	if MatchExecutionDecision(claim, "2", decision.CheckNonce, changed) == nil {
		t.Fatal("wrong checkpoint")
	}
	changed = decision
	changed.CheckedAt = claim.LeaseExpiresAt
	if MatchExecutionDecision(claim, "2", decision.CheckNonce, changed) == nil {
		t.Fatal("expiry equality extended a permit")
	}
	if _, err := DecodeReport(codecVector(t, "Report")); err != nil {
		t.Fatal("valid retained-terminal report")
	}
	result, err := DecodeResultReceipt(codecVector(t, "ResultReceipt"))
	if err != nil || MatchResultReceipt(claim, result) != nil {
		t.Fatal("valid original result receipt")
	}
	if _, err := DecodeRetainedStatus(codecVector(t, "RetainedStatus")); err != nil {
		t.Fatal("valid retained source status")
	}
}

func TestExecutionDTOsRejectShortLeaseAndInventedUnknownEvidence(t *testing.T) {
	claim := string(codecVector(t, "Claimed"))
	for _, raw := range []string{
		strings.Replace(claim, "23:53:00.999999Z", "23:52:59.999999Z", 1),
		strings.Replace(claim, `"claim_credential_revision": "9223372036854775807"`, `"claim_credential_revision": 1`, 1),
		strings.Replace(claim, `"claim_credential_id":`, `"Claim_credential_id":`, 1),
	} {
		if _, err := DecodeClaim([]byte(raw)); err == nil {
			t.Fatal("invalid claim accepted")
		}
	}
	report := string(codecVector(t, "Report"))
	for _, raw := range []string{
		strings.Replace(report, "TARGET_STATE_CONFLICT", "DENIED", 1),
		strings.Replace(report, "TARGET_STATE_CONFLICT", "OUTCOME_UNKNOWN", 1),
		strings.Replace(report, "RETAINED_TERMINAL", "QUIESCED_INTENT", 1),
	} {
		if _, err := DecodeReport([]byte(raw)); err == nil {
			t.Fatal("operational code or mismatched evidence fabricated terminal report")
		}
	}
}
