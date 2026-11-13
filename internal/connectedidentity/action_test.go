package connectedidentity

import (
	"strings"
	"testing"
)

func TestActionIdentityRequiresItsOwnContract(t *testing.T) {
	for _, role := range []string{"action-worker", "action-broker", "action-setup"} {
		value := Identity{Role: role, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: ActionContractDigest()}
		record, err := Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		if parsed, err := Parse(record); err != nil || parsed != value {
			t.Fatal("action identity did not round-trip")
		}
		if _, err := Resolve(record, "install"); err == nil {
			t.Fatal("action identity accepted by observation installer")
		}
		value.ContractSHA256 = ""
		if _, err := Encode(value); err == nil {
			t.Fatal("action identity accepted without contract")
		}
	}
}
