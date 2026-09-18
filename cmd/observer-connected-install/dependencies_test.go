package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The privileged first-install command must not acquire steady-work,
// activation, refresh or server-management authority by transitive import.
func TestInstallDependencyClosure(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go toolchain unavailable in executable-only fixture")
	}
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatal("installer dependency closure unavailable", err)
	}
	for _, path := range strings.Fields(string(output)) {
		for _, forbidden := range []string{
			"/internal/connectedstartup", "/internal/uploadloop",
			"/internal/connectedcollector", "/internal/dockercontrol",
			"/internal/kubecontrol",
		} {
			if strings.HasSuffix(path, forbidden) {
				t.Fatal("installer imported runtime or management authority", path)
			}
		}
	}
}
