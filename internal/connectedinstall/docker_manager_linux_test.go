//go:build linux

package connectedinstall

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"strings"
	"testing"
)

func TestDockerPairUsesFixedNamesAndRejectsOldEnrollment(t *testing.T) {
	c, u, s, ok := richProcessesDockerPair.principalNames()
	if !ok || c != "hlo-rich-docker-collector" || u != "hlo-rich-docker-uploader" || s != "hlo-rich-docker-read" {
		t.Fatal("Docker principal binding")
	}
	c, u, e, ok := richProcessesDockerPair.units()
	if !ok || c != connectedunits.RichProcessesDockerCollectorUnit || u != connectedunits.RichProcessesDockerUploaderUnit || e != "home-lab-observer-connected-rich-docker-enrollment.service" {
		t.Fatal("Docker unit binding")
	}
	if richProcessesDockerPair.uploaderExecutable(strings.Repeat("a", 64)) != "/opt/home-lab-observer-connected-rich/releases/"+strings.Repeat("a", 64)+"/observer-connected-rich-docker-uploader" {
		t.Fatal("Docker executable binding")
	}
	read := func(context.Context) (map[string]string, error) {
		t.Fatal("invalid invocation reached manager")
		return nil, ErrUnsafe
	}
	for _, old := range []string{"/bin/observer-connected-uploader", "/bin/observer-connected-rich-uploader"} {
		if _, err := runEnrollmentForPair(context.Background(), nil, old, "enroll", nil, connectedunits.EnrollmentInput{}, read, richProcessesDockerPair); err != ErrUnsafe {
			t.Fatal("old enrollment executable admitted")
		}
	}
	if !knownInstalledUnit(connectedunits.RichProcessesDockerReaderUnit, false) || knownInstalledUnit(connectedunits.RichProcessesDockerReaderUnit+".extra", false) {
		t.Fatal("reader service allowlist")
	}
}
