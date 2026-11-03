package actionprotocol

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRetainedLaneStatesConsumeReviewedPairedCases(t *testing.T) {
	receiptVector(t) // Independently pinned complete shared source fixture.
	raw, err := os.ReadFile("testdata/paired-container-execution-v1.json")
	var fixture struct {
		Cases []struct {
			Name     string `json:"name"`
			Raw      string `json:"raw"`
			Expected string `json:"expected"`
		} `json:"post_finalization_retained_cases"`
	}
	if err != nil || json.Unmarshal(raw, &fixture) != nil || len(fixture.Cases) != 7 {
		t.Fatal("paired retained cases absent")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if c.Expected != "accept" && c.Expected != "refuse" {
				t.Fatal("unsupported paired expectation")
			}
			_, err := DecodeRetainedStatus([]byte(c.Raw))
			if (err == nil) != (c.Expected == "accept") {
				t.Fatal("observational retained state contract mismatch")
			}
		})
	}
}
