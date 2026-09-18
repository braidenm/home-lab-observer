package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The collector must never acquire enrollment, credential, upload, or installer
// dependencies merely because those packages coexist in this repository.
func TestCollectorDependencyClosure(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields("connectedcompat connectedidentity connectedprocess connectedprofile connectedruntime connectedstatus observation numerichost remoteprojection ownerfs sharedhandoff") {
		allowed[name] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/observer-connected-collector")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.WaitDelay = time.Second
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("dependency inspection failed: %v", err)
	}
	for _, path := range strings.Fields(string(data)) {
		if name, ok := strings.CutPrefix(path, "github.com/braidenm/home-lab-observer/internal/"); ok && !allowed[name] {
			t.Fatalf("unexpected collector dependency: %s", name)
		}
	}
}
