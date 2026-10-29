package connectedidentity

import (
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestRichRolesRequireExactDefinitionContract(t *testing.T) {
	for _, role := range []string{"rich-collector", "rich-uploader", "rich-install"} {
		i := Identity{Role: role, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDefinitionSHA256}
		record, err := Encode(i)
		if err != nil || !i.HasKnownContract() {
			t.Fatalf("%s encode = %q, %v", role, record, err)
		}
		parsed, err := Parse(record)
		if err != nil || parsed != i {
			t.Fatalf("%s parsed = %#v, %v", role, parsed, err)
		}
		for _, bad := range []string{"", connectedcompat.Digest(), strings.Repeat("f", 64)} {
			i.ContractSHA256 = bad
			if i.Validate() != ErrInvalid {
				t.Fatalf("%s admitted contract %q", role, bad)
			}
		}
	}
}

func TestNumericRolesDoNotAdmitRichDefinition(t *testing.T) {
	for _, role := range []string{"collector", "uploader", "install"} {
		numeric := Identity{Role: role, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedcompat.Digest()}
		original, err := Encode(numeric)
		if err != nil {
			t.Fatal(err)
		}
		numeric.ContractSHA256 = connectedprofile.RichProcessesDefinitionSHA256
		if numeric.Validate() != ErrInvalid {
			t.Fatalf("numeric %s admitted rich contract", role)
		}
		numeric.ContractSHA256 = connectedcompat.Digest()
		again, _ := Encode(numeric)
		if again != original {
			t.Fatal("numeric identity bytes changed")
		}
	}
	for _, role := range []string{"rich-docker-reader", "docker", "collector-rich"} {
		i := Identity{Role: role, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDefinitionSHA256}
		if i.Validate() != ErrInvalid {
			t.Fatalf("future/arbitrary role admitted: %s", role)
		}
	}
}
