//go:build linux

package connectedtransitionview

import (
	"os/exec"
	"strings"
	"testing"
)

func TestTransitionViewHasNoInstallerOrWorkerAuthority(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatal("transition view dependency closure unavailable", err)
	}
	for _, path := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"/internal/connectedcredential", "/internal/connectedenroll",
			"/internal/connectedinstall", "/internal/connectedstartup",
			"/internal/platformtransport", "/internal/uploadledger",
			"/internal/uploadloop",
		} {
			if strings.HasSuffix(path, forbidden) {
				t.Fatal("read-only transition view imported mutation or secret authority", path)
			}
		}
	}
}
