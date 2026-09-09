//go:build windows

package background

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTaskTemplateIsAcceptedInMemoryWithoutRegistration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Home Lab Observer")
	adapter, err := newPlatformAdapter(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := adapter.registration(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if !validTaskXML(string(definition.content), definition.content) {
		t.Fatal("generated task is not its own exact canonical definition")
	}
	path := filepath.Join(t.TempDir(), "task.xml")
	roundTrip := filepath.Join(t.TempDir(), "roundtrip.xml")
	if err := os.WriteFile(path, definition.content, 0o600); err != nil {
		t.Fatal(err)
	}
	powershell, err := trustedManagerExecutable("powershell.exe")
	if err != nil {
		t.Skip(err)
	}
	script := `$xml=[IO.File]::ReadAllText($args[0]); $service=New-Object -ComObject 'Schedule.Service'; $service.Connect(); $task=$service.NewTask(0); $task.XmlText=$xml; if ($task.Principal.LogonType -ne 3 -or $task.Principal.RunLevel -ne 0 -or $task.Settings.ExecutionTimeLimit -ne 'PT0S') { exit 9 }; [IO.File]::WriteAllText($args[1],$task.XmlText,(New-Object Text.UTF8Encoding($false)))`
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-Command", script, path, roundTrip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Task Scheduler rejected generated XML: %v: %s", err, output)
	}
	normalized, err := os.ReadFile(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	if !validTaskXML(string(normalized), definition.content) {
		t.Fatal("canonical matcher rejected Task Scheduler's in-memory XML round trip")
	}
	malicious := strings.Replace(string(definition.content), "</Actions>", "<Exec><Command>cmd.exe</Command></Exec></Actions>", 1)
	if validTaskXML(malicious, definition.content) {
		t.Fatal("extra action was accepted as owned")
	}
}

func TestWindowsBackgroundRejectsCommandAndRemotePathSyntax(t *testing.T) {
	for _, root := range []string{`C:\safe&calc`, `C:\safe'quote`, `\\server\share\observer`, `\\?\C:\observer`, `\\.\C:\observer`} {
		if strings.HasPrefix(root, `C:`) {
			if _, err := newPlatformAdapter(nil, root); err == nil {
				t.Fatalf("unsafe task command root accepted: %q", root)
			}
		} else if _, err := secureAbsolutePath(root, false); err == nil {
			t.Fatalf("remote/device root accepted: %q", root)
		}
	}
}

func TestWindowsNormalStopUsesOnlyGracefulLifecycle(t *testing.T) {
	stopper := &fakeStopper{}
	adapter := &windowsAdapter{root: `C:\Observer`}
	if err := adapter.stop(context.Background(), false, stopper, Settings{StateDir: `C:\State`}); err != nil {
		t.Fatal(err)
	}
	if stopper.calls != 1 {
		t.Fatalf("graceful calls = %d", stopper.calls)
	}
}
