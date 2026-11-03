package actionprotocol

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func recoveryVectors(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	receiptVector(t) // Source hash is independently pinned by the receipt test.
	raw, err := os.ReadFile("testdata/paired-container-execution-v1.json")
	var fixture struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("paired recovery fixture")
	}
	return fixture.Objects
}

func TestRecoveryTypedObjectsMatchPairedFixture(t *testing.T) {
	v := recoveryVectors(t)
	checks := map[string]func([]byte) error{
		"RecoveryProposal": func(raw []byte) error { _, e := DecodeRecoveryProposal(raw); return e },
		"PrepareRequest":   func(raw []byte) error { _, e := DecodePrepareRequest(raw); return e },
		"Approval":         func(raw []byte) error { _, e := DecodeApproval(raw); return e },
		"RecoveryCheck":    func(raw []byte) error { _, e := DecodeRecoveryCheck(raw); return e },
		"RecoveryDecision": func(raw []byte) error { _, e := DecodeRecoveryDecision(raw); return e },
		"FinalizeRequest":  func(raw []byte) error { _, e := DecodeFinalizeRequest(raw); return e },
		"Finalization":     func(raw []byte) error { _, e := DecodeFinalization(raw); return e },
		"RecoveryView":     func(raw []byte) error { _, e := DecodeRecoveryView(raw); return e },
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if name == "RecoveryView" {
				if check(v[name]) == nil {
					t.Fatal("MAX sizing view became reachable release")
				}
				v[name] = reachableRecoveryView(t, v[name])
			}
			if check(v[name]) != nil {
				t.Fatal("positive paired object refused")
			}
			for _, raw := range []string{
				strings.Replace(string(v[name]), "{", `{"unknown-secret-canary":0,`, 1),
				strings.Replace(string(v[name]), `"recovery_id"`, `"Recovery_id"`, 1),
			} {
				if raw != string(v[name]) && check([]byte(raw)) == nil {
					t.Fatal("closed recovery fields widened")
				}
			}
		})
	}
}

func TestRecoveryApprovalAndReleaseRequireExactBinding(t *testing.T) {
	v := recoveryVectors(t)
	a, err := DecodeApproval(v["Approval"])
	d, e2 := DecodeRecoveryDecision(v["RecoveryDecision"])
	f, e3 := DecodeFinalization(v["Finalization"])
	a.LaneRevision = "3"
	d.Approval.LaneRevision = "3"
	f.ReleasedLaneRevision = "4"
	if err != nil || e2 != nil || e3 != nil || MatchRecoveryDecision(a, d.CheckNonce, d) != nil || MatchFinalization(a, f.ACKDigest, f) != nil {
		t.Fatal("paired exact binding")
	}
	bad := d
	bad.Approval.OwnerUserID = "usr_" + strings.Repeat("2", 32)
	if MatchRecoveryDecision(a, d.CheckNonce, bad) == nil {
		t.Fatal("owner substituted")
	}
	bad = d
	bad.CheckedAt = a.ExpiresAt
	if MatchRecoveryDecision(a, d.CheckNonce, bad) == nil {
		t.Fatal("equality is expired")
	}
	bad = d
	bad.Decision = "DENIED"
	if MatchRecoveryDecision(a, d.CheckNonce, bad) == nil {
		t.Fatal("denied became permit")
	}
	f.ACKDigest = strings.Repeat("b", 64)
	if MatchFinalization(a, strings.Repeat("a", 64), f) == nil {
		t.Fatal("ACK substituted")
	}
	for _, change := range []struct{ old, new string }{
		{`"approval_revision": "2"`, `"approval_revision": "9"`},
		{`"previous_prepare_id": "00000000000000000000000000000004"`, `"previous_prepare_id": null`},
		{`"expires_at": "9999-12-31T23:59:00.999999Z"`, `"expires_at": "9999-12-31T23:55:00.999999Z"`},
	} {
		if _, err := DecodeApproval([]byte(strings.Replace(string(v["Approval"]), change.old, change.new, 1))); err == nil {
			t.Fatal("invalid approval history/chronology accepted")
		}
	}
}

func reachableRecoveryView(t *testing.T, raw []byte) json.RawMessage {
	t.Helper()
	var v RecoveryView
	if json.Unmarshal(raw, &v) != nil || v.CurrentApproval == nil || v.Finalization == nil {
		t.Fatal("sizing view fixture")
	}
	v.LaneRevision = "4"
	v.CurrentApproval.LaneRevision = "3"
	v.Finalization.ReleasedLaneRevision = "4"
	result, err := json.Marshal(v)
	if err != nil {
		t.Fatal("reachable private test copy")
	}
	return result
}

func TestFinalizationMustAdvanceExactlyOneCheckedLaneRevision(t *testing.T) {
	v := recoveryVectors(t)
	a, _ := DecodeApproval(v["Approval"])
	f, _ := DecodeFinalization(v["Finalization"])
	if MatchFinalization(a, f.ACKDigest, f) == nil {
		t.Fatal("MAX sizing pair became reachable finalization")
	}
	a.LaneRevision = "3"
	f.ReleasedLaneRevision = "4"
	if MatchFinalization(a, f.ACKDigest, f) != nil {
		t.Fatal("exact 3 to 4 release refused")
	}
	for _, revision := range []string{"2", "3", "5", "9223372036854775807"} {
		f.ReleasedLaneRevision = revision
		if MatchFinalization(a, f.ACKDigest, f) == nil {
			t.Fatal("unchanged/jumped lane release accepted")
		}
	}
}
