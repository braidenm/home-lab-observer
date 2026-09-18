package connectedstatus

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStatusKeepsUploaderAuthorityOutOfCollectorClosure(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatal("status dependency closure unavailable", err)
	}
	for _, path := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"/internal/connectedactivation",
			"/internal/connectedcredential",
			"/internal/connectedenroll",
			"/internal/connectedstartup",
			"/internal/platformtransport",
		} {
			if strings.HasSuffix(path, forbidden) {
				t.Fatal("status imported uploader authority", path)
			}
		}
	}
}
