package connectedunits

import (
	"slices"
	"strings"
	"testing"
)

func TestExistingLedgerUsesOnlyOfflineFinalLedgerProfile(t *testing.T) {
	c := fixture()
	input := EnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	pristine, e1 := RenderEnrollmentProperties(input, ValidateLedger)
	existing, e2 := RenderEnrollmentProperties(input, ValidateExistingLedger)
	if e1 != nil || e2 != nil || !slices.Equal(pristine.Properties, existing.Properties) || existing.Executable != pristine.Executable || !slices.Equal(existing.Args, []string{"validate-existing-ledger"}) {
		t.Fatal("existing validation changed offline authority")
	}
	text := strings.Join(existing.Properties, "\n")
	for _, forbidden := range []string{"LoadCredential=", "/state/enrollment", "/handoff", "IPAddressAllow=", "RestrictAddressFamilies="} {
		if strings.Contains(text, forbidden) {
			t.Fatal("existing validation acquired extra authority")
		}
	}
}
