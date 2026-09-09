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
	if !strings.Contains(string(definition.content), `-WindowStyle Hidden`) {
		t.Fatal("generated task does not hide its PowerShell host window")
	}
	if !validTaskXML(string(definition.content), definition.content) {
		t.Fatal("generated task is not its own exact canonical definition")
	}
	path := filepath.Join(t.TempDir(), "task.xml")
	roundTrip := filepath.Join(t.TempDir(), "roundtrip.xml")
	expectedArgumentsPath := filepath.Join(t.TempDir(), "expected-arguments.txt")
	expectedArguments := windowsPowerShellArguments(filepath.Join(root, "bin", launcherName()), root)
	withoutRunLevel := []byte(strings.Replace(string(definition.content), `<RunLevel>LeastPrivilege</RunLevel>`, "", 1))
	if err := os.WriteFile(path, withoutRunLevel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(expectedArgumentsPath, []byte(expectedArguments), 0o600); err != nil {
		t.Fatal(err)
	}
	powershell, err := trustedManagerExecutable("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	script := `$xml=[IO.File]::ReadAllText($args[0]); $expectedArguments=[IO.File]::ReadAllText($args[2]); $service=New-Object -ComObject 'Schedule.Service'; $service.Connect(); $task=$service.NewTask(0); $task.XmlText=$xml; $task.RegistrationInfo.URI='\Home Lab Observer'; $actualArguments=[string]$task.Actions.Item(1).Arguments; if ($task.Principal.LogonType -ne 3 -or $task.Principal.RunLevel -ne 0 -or $task.Settings.ExecutionTimeLimit -ne 'PT0S') { exit 9 }; if ($actualArguments -cne $expectedArguments) { exit 10 }; if ($actualArguments -match '&(?:amp|apos|quot|lt|gt);') { exit 11 }; [IO.File]::WriteAllText($args[1],$task.XmlText,(New-Object Text.UTF8Encoding($false)))`
	scriptPath := filepath.Join(t.TempDir(), "verify-task.ps1")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, path, roundTrip, expectedArgumentsPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Task Scheduler rejected generated XML: %v: %s", err, output)
	}
	normalized, err := os.ReadFile(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	if !validTaskXML(string(normalized), definition.content) {
		want, _ := canonicalTaskXML(definition.content)
		got, _ := canonicalTaskXML(normalized)
		for index := 0; index < len(want) && index < len(got); index++ {
			if want[index] != got[index] {
				t.Fatalf("canonical matcher rejected Task Scheduler XML at %d: want %q got %q; want tail %q got tail %q", index, want[index], got[index], want[index:], got[index:])
			}
		}
		t.Fatalf("canonical matcher rejected Task Scheduler XML token lengths: want %d got %d", len(want), len(got))
	}
	registered := string(normalized)
	if !strings.Contains(registered, `<URI>\Home Lab Observer</URI>`) {
		t.Fatal("Task Scheduler in-memory round trip did not expose registration URI metadata")
	}
	if !validTaskXML(registered, definition.content) {
		t.Fatal("registered task URI metadata was not normalized")
	}
	for _, changed := range []string{
		strings.Replace(registered, `\Home Lab Observer</URI>`, `\Another Task</URI>`, 1),
		strings.Replace(registered, "</RegistrationInfo>", `<URI>\Home Lab Observer</URI></RegistrationInfo>`, 1),
		strings.Replace(registered, "</RegistrationInfo>", `<Author>unexpected</Author></RegistrationInfo>`, 1),
		strings.Replace(registered, `<URI>\Home Lab Observer</URI>`, `<URI source="unexpected">\Home Lab Observer</URI>`, 1),
		strings.Replace(registered, `<URI>\Home Lab Observer</URI>`, `<URI><Value>\Home Lab Observer</Value></URI>`, 1),
	} {
		if validTaskXML(changed, definition.content) {
			t.Fatal("unknown or mismatched registration metadata was accepted as owned")
		}
	}
	malicious := strings.Replace(string(definition.content), "</Actions>", "<Exec><Command>cmd.exe</Command></Exec></Actions>", 1)
	if validTaskXML(malicious, definition.content) {
		t.Fatal("extra action was accepted as owned")
	}
}

func TestWindowsSavedTaskMayOmitOnlyExactKnownDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Home Lab Observer")
	adapter, err := newPlatformAdapter(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := adapter.registration(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	original := string(definition.content)
	cases := []struct {
		name       string
		element    string
		occurrence int
		nonDefault string
	}{
		{name: "trigger enabled", element: `<Enabled>true</Enabled>`, occurrence: 1, nonDefault: `<Enabled>false</Enabled>`},
		{name: "least privilege", element: `<RunLevel>LeastPrivilege</RunLevel>`, occurrence: 1, nonDefault: `<RunLevel>HighestAvailable</RunLevel>`},
		{name: "hard terminate", element: `<AllowHardTerminate>true</AllowHardTerminate>`, occurrence: 1, nonDefault: `<AllowHardTerminate>false</AllowHardTerminate>`},
		{name: "network not required", element: `<RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>`, occurrence: 1, nonDefault: `<RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>`},
		{name: "demand start", element: `<AllowStartOnDemand>true</AllowStartOnDemand>`, occurrence: 1, nonDefault: `<AllowStartOnDemand>false</AllowStartOnDemand>`},
		{name: "task enabled", element: `<Enabled>true</Enabled>`, occurrence: 2, nonDefault: `<Enabled>false</Enabled>`},
		{name: "visible", element: `<Hidden>false</Hidden>`, occurrence: 1, nonDefault: `<Hidden>true</Hidden>`},
		{name: "not idle only", element: `<RunOnlyIfIdle>false</RunOnlyIfIdle>`, occurrence: 1, nonDefault: `<RunOnlyIfIdle>true</RunOnlyIfIdle>`},
		{name: "remote app allowed", element: `<DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession>`, occurrence: 1, nonDefault: `<DisallowStartOnRemoteAppSession>true</DisallowStartOnRemoteAppSession>`},
		{name: "no wake", element: `<WakeToRun>false</WakeToRun>`, occurrence: 1, nonDefault: `<WakeToRun>true</WakeToRun>`},
		{name: "priority", element: `<Priority>7</Priority>`, occurrence: 1, nonDefault: `<Priority>6</Priority>`},
	}

	withoutDefaults := original
	for index := len(cases) - 1; index >= 0; index-- {
		test := cases[index]
		withoutDefaults = replaceOccurrence(t, withoutDefaults, test.element, test.occurrence, "")
	}
	if !validTaskXML(withoutDefaults, definition.content) {
		t.Fatal("saved task with only known default-valued fields omitted was rejected")
	}
	registeredWithoutDefaults := strings.Replace(withoutDefaults, "</RegistrationInfo>", `<URI>\Home Lab Observer</URI></RegistrationInfo>`, 1)
	if !validTaskXML(registeredWithoutDefaults, definition.content) {
		t.Fatal("saved task with registration URI and only known defaults omitted was rejected")
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			nonDefault := replaceOccurrence(t, original, test.element, test.occurrence, test.nonDefault)
			if validTaskXML(nonDefault, definition.content) {
				t.Fatal("non-default value was accepted as owned")
			}
			duplicate := replaceOccurrence(t, original, test.element, test.occurrence, test.element+test.element)
			if validTaskXML(duplicate, definition.content) {
				t.Fatal("duplicate default-valued field was accepted as owned")
			}
		})
	}

	for _, changed := range []string{
		strings.Replace(original, `<Priority>7</Priority>`, `<Priority source="unexpected">7</Priority>`, 1),
		strings.Replace(original, `<Priority>7</Priority>`, `<Priority><Value>7</Value></Priority>`, 1),
	} {
		if validTaskXML(changed, definition.content) {
			t.Fatal("attribute or nested content on a default-valued field was accepted")
		}
	}
}

func replaceOccurrence(t *testing.T, value, old string, occurrence int, replacement string) string {
	t.Helper()
	start := 0
	for index := 1; index <= occurrence; index++ {
		offset := strings.Index(value[start:], old)
		if offset < 0 {
			t.Fatalf("fixture occurrence %d of %q is missing", occurrence, old)
		}
		start += offset
		if index == occurrence {
			return value[:start] + replacement + value[start+len(old):]
		}
		start += len(old)
	}
	return value
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
