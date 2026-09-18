//go:build linux

package connectedenroll

import (
	"os/exec"
	"strings"
	"testing"
)

// The short-lived offline worker must not become an installer or steady
// uploader simply because another package adds a transitive import.
func TestEnrollmentDependencyClosure(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatal("enrollment dependency closure unavailable", err)
	}
	for _, path := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"/internal/connectedinstall",
			"/internal/connectedstartup",
			"/internal/uploadloop",
		} {
			if strings.HasSuffix(path, forbidden) {
				t.Fatal("offline enrollment imported installer or steady-work authority", path)
			}
		}
	}
}
