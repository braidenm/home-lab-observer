//go:build linux

package connectedstartup

import (
	"os/exec"
	"strings"
	"testing"
)

// The pre-commit gate must not acquire the ordinary uploader's secret, ledger,
// enrollment, or upload authority through a transitive dependency.
func TestStartupDependencyClosureExcludesOrdinaryWork(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatal("startup dependency closure unavailable", err)
	}
	for _, path := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"/internal/connectedcredential",
			"/internal/connectedenroll",
			"/internal/connectedinstall",
			"/internal/enrollmentcoord",
			"/internal/enrollmentstore",
			"/internal/ledgerwitness",
			"/internal/ownerfs",
			"/internal/platformtransport",
			"/internal/sharedhandoff",
			"/internal/uploadledger",
			"/internal/uploadloop",
			"/internal/uploadstate",
		} {
			if strings.HasSuffix(path, forbidden) {
				t.Fatal("startup gate imported ordinary uploader authority", path)
			}
		}
	}
}
