//go:build windows

package background

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const managerIdentity = `\Home Lab Observer`

const taskXMLNamespace = "http://schemas.microsoft.com/windows/2004/02/mit/task"

type windowsAdapter struct {
	runner commandRunner
	root   string
}

type taskChildSpan struct {
	name       string
	start, end int
}

func launcherName() string { return "observer.cmd" }

func newPlatformAdapter(runner commandRunner, root string) (platformAdapter, error) {
	if strings.ContainsAny(root, "'\"%`!^&|<>()\r\n") {
		return nil, coded(CodeInvalidSettings, errors.New("Windows install root contains command-line metacharacters unsupported by background mode"))
	}
	return &windowsAdapter{runner: runner, root: root}, nil
}

func (a *windowsAdapter) registration(Settings) (registration, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return registration{}, err
	}
	sid := tokenUser.User.Sid.String()
	launcher := filepath.Join(a.root, "bin", launcherName())
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return registration{}, err
	}
	powershell := filepath.Join(systemDirectory, "WindowsPowerShell", "v1.0", "powershell.exe")
	argument := windowsPowerShellArguments(launcher, a.root)
	content := fmt.Sprintf(`<?xml version="1.0"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Home Lab Observer per-user session profile</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>%s</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Owner"><UserId>%s</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>true</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings><AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>false</Hidden><RunOnlyIfIdle>false</RunOnlyIfIdle><DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession><UseUnifiedSchedulingEngine>false</UseUnifiedSchedulingEngine><WakeToRun>false</WakeToRun><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><Priority>7</Priority><RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure></Settings>
  <Actions Context="Owner"><Exec><Command>%s</Command><Arguments>%s</Arguments></Exec></Actions>
</Task>
`, sid, sid, xmlEscape(powershell), xmlEscape(argument))
	return registration{fileName: "task.xml", content: []byte(content)}, nil
}

func windowsPowerShellArguments(launcher, root string) string {
	return fmt.Sprintf(`-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -Command "& '%s' background run --install-root '%s'"`, launcher, root)
}

func (a *windowsAdapter) available(ctx context.Context) bool {
	_, err := a.runner.run(ctx, command{name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; try { $service=New-Object -ComObject 'Schedule.Service'; $service.Connect(); exit 0 } catch { exit 20 }`}})
	return err == nil
}

func (a *windowsAdapter) inspect(ctx context.Context, expected registration) (managerState, error) {
	stateResult, stateErr := a.runner.run(ctx, command{name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; try { $service=New-Object -ComObject 'Schedule.Service'; $service.Connect(); $task=$service.GetFolder('\').GetTask('\Home Lab Observer'); if ($task.State -eq 4) { exit 10 }; exit 0 } catch { if (($_.Exception.HResult -band 0xffff) -eq 2) { exit 11 }; exit 20 }`}})
	if stateResult.exitCode == 20 || (stateErr != nil && stateResult.exitCode != 10 && stateResult.exitCode != 11) {
		return managerState{}, nil
	}
	state := managerState{available: true}
	if stateResult.exitCode == 11 {
		return state, nil
	}
	state.registered = true
	state.running = stateResult.exitCode == 10
	result, err := a.runner.run(ctx, command{name: "schtasks.exe", args: []string{"/Query", "/TN", managerIdentity, "/XML"}, capture: true})
	if err != nil {
		return state, err
	}
	if !validTaskXML(result.output, expected.content) {
		return state, coded(CodeRegistrationMismatch, errors.New("existing scheduled task is not managed by this observer"))
	}
	return state, nil
}

func validTaskXML(actual string, expected []byte) bool {
	want, err := canonicalTaskXML(expected)
	if err != nil {
		return false
	}
	got, err := canonicalTaskXML([]byte(actual))
	return err == nil && strings.Join(got, "\x00") == strings.Join(want, "\x00")
}

func canonicalTaskXML(value []byte) ([]string, error) {
	normalized, err := normalizeTaskXMLBytes(value)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(normalized))
	var tokens []string
	var elements []xml.Name
	registrationURISeen := false
	defaultFieldsSeen := make(map[string]bool)
	rootTask := false
	directChildCounts := make(map[string]int)
	var directChildren []taskChildSpan
	openDirectChild := taskChildSpan{start: -1}
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(elements) != 0 {
					return nil, errors.New("task XML element nesting is incomplete")
				}
				if rootTask {
					return canonicalizeDirectTaskChildren(tokens, directChildren, directChildCounts)
				}
				return tokens, nil
			}
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if len(elements) == 0 {
				if len(tokens) != 0 {
					return nil, errors.New("task XML contains multiple roots")
				}
				rootTask = typed.Name.Space == taskXMLNamespace && typed.Name.Local == "Task"
			} else if rootTask && len(elements) == 1 {
				if typed.Name.Space != taskXMLNamespace || !allowedDirectTaskChild(typed.Name.Local) {
					return nil, errors.New("task XML contains an unknown direct task child")
				}
				directChildCounts[typed.Name.Local]++
				if directChildCounts[typed.Name.Local] != 1 {
					return nil, errors.New("task XML contains a duplicate direct task child")
				}
				openDirectChild = taskChildSpan{name: typed.Name.Local, start: len(tokens)}
			}
			if isTaskRegistrationURI(elements, typed.Name) {
				if registrationURISeen {
					return nil, errors.New("task XML contains duplicate registration URI metadata")
				}
				if err := consumeExactSimpleTaskElement(decoder, typed, managerIdentity); err != nil {
					return nil, errors.New("task XML registration URI does not match the managed task")
				}
				registrationURISeen = true
				continue
			}
			if path, defaultValue, ok := normalizedTaskDefault(elements, typed.Name); ok {
				if defaultFieldsSeen[path] {
					return nil, errors.New("task XML contains duplicate default-valued metadata")
				}
				if err := consumeExactSimpleTaskElement(decoder, typed, defaultValue); err != nil {
					return nil, errors.New("task XML contains changed default-valued metadata")
				}
				defaultFieldsSeen[path] = true
				continue
			}
			attributes := make([]string, 0, len(typed.Attr))
			for _, attr := range typed.Attr {
				attributes = append(attributes, attr.Name.Space+"|"+attr.Name.Local+"="+attr.Value)
			}
			sort.Strings(attributes)
			tokens = append(tokens, "<"+typed.Name.Space+"|"+typed.Name.Local+" "+strings.Join(attributes, " ")+">")
			elements = append(elements, typed.Name)
		case xml.EndElement:
			if len(elements) == 0 || elements[len(elements)-1] != typed.Name {
				return nil, errors.New("task XML element nesting is invalid")
			}
			tokens = append(tokens, "</"+typed.Name.Space+"|"+typed.Name.Local+">")
			if rootTask && len(elements) == 2 {
				openDirectChild.end = len(tokens)
				directChildren = append(directChildren, openDirectChild)
				openDirectChild = taskChildSpan{start: -1}
			}
			elements = elements[:len(elements)-1]
		case xml.CharData:
			if text := strings.TrimSpace(string(typed)); text != "" {
				if rootTask && len(elements) == 1 {
					return nil, errors.New("task XML contains direct task text")
				}
				tokens = append(tokens, "="+text)
			}
		}
	}
}

func allowedDirectTaskChild(name string) bool {
	switch name {
	case "RegistrationInfo", "Triggers", "Settings", "Data", "Principals", "Actions":
		return true
	default:
		return false
	}
}

// taskType uses xs:all, so Task Scheduler may persist the direct children in
// any order. Normalize that one confirmed schema boundary while keeping each
// entire child subtree, including its order, values and attributes, exact.
func canonicalizeDirectTaskChildren(tokens []string, children []taskChildSpan, counts map[string]int) ([]string, error) {
	if len(tokens) < 2 || counts["Actions"] != 1 || len(children) == 0 {
		return nil, errors.New("task XML is missing its required action")
	}
	cursor := 1
	for _, child := range children {
		if child.start != cursor || child.end <= child.start || child.end > len(tokens)-1 {
			return nil, errors.New("task XML direct child boundaries are invalid")
		}
		cursor = child.end
	}
	if cursor != len(tokens)-1 {
		return nil, errors.New("task XML contains content outside direct task children")
	}
	sort.Slice(children, func(left, right int) bool { return children[left].name < children[right].name })
	canonical := make([]string, 0, len(tokens))
	canonical = append(canonical, tokens[0])
	for _, child := range children {
		canonical = append(canonical, tokens[child.start:child.end]...)
	}
	canonical = append(canonical, tokens[len(tokens)-1])
	return canonical, nil
}

// Task Scheduler omits these exact default-valued fields when it persists a
// task. Normalize only the documented schema defaults observed in the saved
// task, plus the effective least-privilege RunLevel default. A present field
// must still have the exact expected value and may occur only once.
func normalizedTaskDefault(parents []xml.Name, current xml.Name) (string, string, bool) {
	path, ok := exactTaskPath(parents, current)
	if !ok {
		return "", "", false
	}
	switch path {
	case "Task/Triggers/LogonTrigger/Enabled",
		"Task/Settings/AllowHardTerminate",
		"Task/Settings/AllowStartOnDemand",
		"Task/Settings/Enabled":
		return path, "true", true
	case "Task/Settings/RunOnlyIfNetworkAvailable",
		"Task/Settings/Hidden",
		"Task/Settings/RunOnlyIfIdle",
		"Task/Settings/DisallowStartOnRemoteAppSession",
		"Task/Settings/WakeToRun":
		return path, "false", true
	case "Task/Principals/Principal/RunLevel":
		return path, "LeastPrivilege", true
	case "Task/Settings/Priority":
		return path, "7", true
	default:
		return "", "", false
	}
}

func exactTaskPath(parents []xml.Name, current xml.Name) (string, bool) {
	names := make([]string, 0, len(parents)+1)
	for _, element := range append(append([]xml.Name(nil), parents...), current) {
		if element.Space != taskXMLNamespace {
			return "", false
		}
		names = append(names, element.Local)
	}
	return strings.Join(names, "/"), true
}

func consumeExactSimpleTaskElement(decoder *xml.Decoder, start xml.StartElement, expected string) error {
	if len(start.Attr) != 0 {
		return errors.New("task XML metadata contains attributes")
	}
	var contents strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.CharData:
			contents.Write(typed)
		case xml.EndElement:
			if typed.Name != start.Name || strings.TrimSpace(contents.String()) != expected {
				return errors.New("task XML metadata value changed")
			}
			return nil
		default:
			return errors.New("task XML metadata is not a simple value")
		}
	}
}

func isTaskRegistrationURI(parents []xml.Name, current xml.Name) bool {
	if current.Space != taskXMLNamespace || current.Local != "URI" || len(parents) != 2 {
		return false
	}
	return parents[0].Space == taskXMLNamespace && parents[0].Local == "Task" &&
		parents[1].Space == taskXMLNamespace && parents[1].Local == "RegistrationInfo"
}

func normalizeTaskXMLBytes(value []byte) ([]byte, error) {
	if len(value) > 128<<10 {
		return nil, errors.New("task XML exceeds its limit")
	}
	var text string
	if len(value) >= 2 && ((value[0] == 0xff && value[1] == 0xfe) || (value[0] == 0xfe && value[1] == 0xff)) {
		little := value[0] == 0xff
		units := make([]uint16, 0, (len(value)-2)/2)
		for index := 2; index+1 < len(value); index += 2 {
			if little {
				units = append(units, uint16(value[index])|uint16(value[index+1])<<8)
			} else {
				units = append(units, uint16(value[index])<<8|uint16(value[index+1]))
			}
		}
		text = string(utf16.Decode(units))
	} else {
		text = string(value)
	}
	text = strings.Replace(text, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	text = strings.Replace(text, `encoding="utf-16"`, `encoding="UTF-8"`, 1)
	return []byte(text), nil
}

func (a *windowsAdapter) register(ctx context.Context, source string, _ registration) error {
	_, err := a.runner.run(ctx, command{name: "schtasks.exe", args: []string{"/Create", "/TN", managerIdentity, "/XML", source}})
	return err
}

func (a *windowsAdapter) start(ctx context.Context) error {
	_, err := a.runner.run(ctx, command{name: "schtasks.exe", args: []string{"/Run", "/TN", managerIdentity}})
	return err
}

func (a *windowsAdapter) stop(ctx context.Context, force bool, stopper GracefulStopper, settings Settings) error {
	if force {
		_, err := a.runner.run(ctx, command{name: "schtasks.exe", args: []string{"/End", "/TN", managerIdentity}})
		return err
	}
	if stopper == nil {
		return errors.New("graceful lifecycle client is unavailable")
	}
	return stopper.RequestStop(ctx, settings.StateDir)
}

func (a *windowsAdapter) unregister(ctx context.Context, _ string, expected registration) error {
	state, err := a.inspect(ctx, expected)
	if err != nil {
		return err
	}
	if !state.available {
		return errors.New("Task Scheduler is unavailable")
	}
	if state.registered {
		_, err = a.runner.run(ctx, command{name: "schtasks.exe", args: []string{"/Delete", "/TN", managerIdentity, "/F"}})
	}
	return err
}
