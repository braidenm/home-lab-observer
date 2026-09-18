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

// The uploader may use credential and transport adapters but never gains local
// host collectors, Docker, process inspection, installer, or action packages.
func TestUploaderDependencyClosure(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields("connectedcompat connectedcredential connectedprofile connectedactivation connectedruntime connectedstartup observation remoteprojection uploadstate enrollmentcoord ownerfs uploadledger ledgerwitness enrollmentstore platformtransport connectedenroll connectedidentity connectedstatus sharedhandoff uploadloop") {
		allowed[name] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/observer-connected-uploader")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.WaitDelay = time.Second
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("dependency inspection failed: %v", err)
	}
	for _, path := range strings.Fields(string(data)) {
		if name, ok := strings.CutPrefix(path, "github.com/braidenm/home-lab-observer/internal/"); ok && !allowed[name] {
			t.Fatalf("unexpected uploader dependency: %s", name)
		}
	}
}
