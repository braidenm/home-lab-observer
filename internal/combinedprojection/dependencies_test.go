package combinedprojection_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Upload validation must never acquire collectors or Docker transport merely by
// depending on normalized observation types. Inspect production, not test imports.
func TestCombinedProjectionDependencyClosure(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields("combinedprojection workloadprojection remoteprojection observation containerobs/model") {
		allowed["github.com/braidenm/home-lab-observer/internal/"+name] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}} {{.Standard}}", "./internal/combinedprojection")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	cmd.WaitDelay = time.Second
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("dependency inspection failed: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "net/http" || (fields[1] != "true" && !allowed[fields[0]]) {
			t.Fatalf("unexpected combined projection dependency: %s", line)
		}
	}
}
