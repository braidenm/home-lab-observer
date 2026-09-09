//go:build linux

package background

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdTemplateSyntaxAndStopBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "observer $ path")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "bin", "observer")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	adapter := &linuxAdapter{runner: runner, root: root}
	definition, err := adapter.registration(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(definition.content)
	for _, required := range []string{"SendSIGKILL=no", "StandardOutput=null", "StandardError=null", "Restart=on-failure", `ExecStart=:"`, "observer $ path"} {
		if !strings.Contains(text, required) {
			t.Fatalf("unit missing %q", required)
		}
	}
	unit := filepath.Join(t.TempDir(), "home-lab-observer.service")
	if err := os.WriteFile(unit, definition.content, 0o600); err != nil {
		t.Fatal(err)
	}
	if validator, err := exec.LookPath("systemd-analyze"); err == nil {
		if output, err := exec.Command(validator, "verify", unit).CombinedOutput(); err != nil {
			t.Fatalf("systemd rejected generated unit: %v: %s", err, output)
		}
	}
	stopper := &fakeStopper{}
	if err := adapter.stop(context.Background(), false, stopper, Settings{StateDir: "/tmp/observer-state"}); err != nil || stopper.calls != 1 || len(runner.commands) != 0 {
		t.Fatalf("normal stop invoked manager: %v, calls %d/%d", err, stopper.calls, len(runner.commands))
	}
	if err := adapter.stop(context.Background(), true, stopper, Settings{}); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 2 || strings.Join(runner.commands[0].args, " ") != "--user stop --no-block home-lab-observer.service" || strings.Join(runner.commands[1].args, " ") != "--user kill --kill-whom=all --signal=KILL home-lab-observer.service" {
		t.Fatalf("force commands = %#v", runner.commands)
	}
}
