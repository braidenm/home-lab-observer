package connectedidentity

import (
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestDockerIdentityCannotCrossProfileBoundary(t *testing.T) {
	for _, role := range []string{"rich-docker-collector", "rich-docker-reader", "rich-docker-uploader", "rich-docker-install"} {
		i := fixture()
		i.Role = role
		i.ContractSHA256 = connectedprofile.RichProcessesDockerDefinitionSHA256
		b, err := Encode(i)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Parse(b)
		if err != nil || got != i || !got.HasKnownContract() {
			t.Fatal("Docker identity round trip")
		}
		for _, hash := range []string{"", connectedprofile.RichProcessesDefinitionSHA256} {
			i.ContractSHA256 = hash
			if _, err := Encode(i); err != ErrInvalid {
				t.Fatal("Docker role accepted wrong definition")
			}
		}
	}
	i := fixture()
	i.Role = "rich-collector"
	i.ContractSHA256 = connectedprofile.RichProcessesDockerDefinitionSHA256
	if _, err := Encode(i); err != ErrInvalid {
		t.Fatal("old role accepted Docker definition")
	}
}
