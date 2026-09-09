//go:build darwin

package background

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchAgentTemplateSyntaxAndStopBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Observer & local")
	runner := &recordingRunner{}
	adapter := &darwinAdapter{runner: runner, root: root, domain: "gui/501"}
	definition, err := adapter.registration(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(definition.content)
	for _, required := range []string{"<key>RunAtLoad</key><true/>", "<key>SuccessfulExit</key><false/>", "<string>/dev/null</string>", "Observer &amp; local"} {
		if !strings.Contains(text, required) {
			t.Fatalf("plist missing %q", required)
		}
	}
	plist := filepath.Join(t.TempDir(), "observer.plist")
	if err := os.WriteFile(plist, definition.content, 0o600); err != nil {
		t.Fatal(err)
	}
	if validator, err := exec.LookPath("plutil"); err == nil {
		if output, err := exec.Command(validator, "-lint", plist).CombinedOutput(); err != nil {
			t.Fatalf("plutil rejected generated plist: %v: %s", err, output)
		}
	}
	stopper := &fakeStopper{}
	if err := adapter.stop(context.Background(), false, stopper, Settings{StateDir: "/tmp/observer-state"}); err != nil || stopper.calls != 1 || len(runner.commands) != 0 {
		t.Fatalf("normal stop invoked manager: %v, calls %d/%d", err, stopper.calls, len(runner.commands))
	}
}

func TestDarwinForceRestartClearsUnloadedState(t *testing.T) {
	runner := &recordingRunner{}
	adapter := &darwinAdapter{runner: runner, root: "/Applications/Observer", domain: "gui/501"}
	if err := adapter.stop(context.Background(), true, nil, Settings{}); err != nil {
		t.Fatal(err)
	}
	if !adapter.forceUnloaded {
		t.Fatal("force bootout was not recorded")
	}
	if err := adapter.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adapter.forceUnloaded {
		t.Fatal("successful start retained stale unloaded state")
	}
}
