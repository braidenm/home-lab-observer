package connectedprofile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSeparateInstalledWorkerDependencyBoundaries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for role, modules := range map[string]string{
		"collector": "connectedidentity connectedprofile connectedstatus observation numerichost remoteprojection ownerfs sharedhandoff",
		"uploader":  "connectedcredential connectedprofile observation remoteprojection uploadstate enrollmentcoord ownerfs uploadledger enrollmentstore platformtransport connectedenroll connectedidentity connectedstatus sharedhandoff uploadloop",
	} {
		t.Run(role, func(t *testing.T) {
			allowed := map[string]bool{}
			for _, name := range strings.Fields(modules) {
				allowed[name] = true
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			c := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/observer-connected-"+role)
			c.Dir = root
			c.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
			c.WaitDelay = time.Second
			data, err := c.Output()
			if err != nil {
				t.Fatal("dependency inspection failed", err)
			}
			for _, path := range strings.Fields(string(data)) {
				if name, ok := strings.CutPrefix(path, "github.com/braidenm/home-lab-observer/internal/"); ok && !allowed[name] {
					t.Fatalf("unexpected %s dependency: %s", role, name)
				}
			}
		})
	}
}
