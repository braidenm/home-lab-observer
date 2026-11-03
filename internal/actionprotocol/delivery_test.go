package actionprotocol

import (
	"encoding/json"
	"testing"
)

func TestDeliveryRequestCodecsConsumePairedBinding(t *testing.T) {
	claim, err := DecodeClaimRequest(codecVector(t, "ClaimRequest"))
	if err != nil || len(claim.EligibleTargetIDs) != 32 {
		t.Fatal("bounded eligible target filter")
	}
	response, err := DecodeClaimResponse(codecVector(t, "ClaimResponse"))
	if err != nil || response.ClaimNonce != claim.ClaimNonce {
		t.Fatal("claim nonce envelope")
	}
	check, err := DecodeCheckRequest(codecVector(t, "CheckRequest"))
	if err != nil || check.Request != response.Claim.Request || check.ExecutionLeaseID != response.Claim.ExecutionLeaseID {
		t.Fatal("full checkpoint binding")
	}
	claim.EligibleTargetIDs[0], claim.EligibleTargetIDs[1] = claim.EligibleTargetIDs[1], claim.EligibleTargetIDs[0]
	raw, _ := json.Marshal(claim)
	if _, err := DecodeClaimRequest(raw); err == nil {
		t.Fatal("eligibility silently sorted")
	}
	claim.EligibleTargetIDs = []string{}
	raw, _ = json.Marshal(claim)
	if _, err := DecodeClaimRequest(raw); err == nil {
		t.Fatal("empty broker eligibility sent to claim")
	}
}
