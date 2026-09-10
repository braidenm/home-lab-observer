//go:build windows

package background

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const managerIdentity = `\Home Lab Observer`

const taskXMLNamespace = "http://schemas.microsoft.com/windows/2004/02/mit/task"

const (
	maxTaskXMLDepth    = 32
	maxTaskXMLElements = 512
	maxOwnerNameBytes  = 512
)

var compareStringOrdinal = windows.NewLazySystemDLL("kernel32.dll").NewProc("CompareStringOrdinal")

type windowsAdapter struct {
	runner commandRunner
	root   string
}

type taskChildSpan struct {
	name       string
	start, end int
}

type taskElementFrame struct {
	name       xml.Name
	path       string
	start      int
	childStart int
	allGroup   bool
	children   []taskChildSpan
	counts     map[string]int
}

type windowsOwnerIdentity struct {
	sid          string
	qualifiedSAM string
	localSAM     string
}

func launcherName() string { return "observer.cmd" }

func newPlatformAdapter(runner commandRunner, root string) (platformAdapter, error) {
	if strings.ContainsAny(root, "'\"%`!^&|<>()\r\n") {
		return nil, coded(CodeInvalidSettings, errors.New("Windows install root contains command-line metacharacters unsupported by background mode"))
	}
	return &windowsAdapter{runner: runner, root: root}, nil
}

func (a *windowsAdapter) registration(Settings) (registration, error) {
	owner, err := currentWindowsOwnerSID()
	if err != nil {
		return registration{}, err
	}
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
  <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>true</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings><AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>false</Hidden><RunOnlyIfIdle>false</RunOnlyIfIdle><DisallowStartOnRemoteAppSession>false</DisallowStartOnRemoteAppSession><UseUnifiedSchedulingEngine>true</UseUnifiedSchedulingEngine><WakeToRun>false</WakeToRun><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><Priority>7</Priority><RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure></Settings>
  <Actions Context="Owner"><Exec><Command>%s</Command><Arguments>%s</Arguments></Exec></Actions>
</Task>
`, owner.sid, owner.sid, xmlEscape(powershell), xmlEscape(argument))
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
	owner, err := a.currentWindowsOwnerIdentity(ctx)
	if err != nil {
		return state, errors.New("resolve current Windows task owner")
	}
	if !validTaskXMLForOwner(result.output, expected.content, owner) {
		return state, coded(CodeRegistrationMismatch, errors.New("existing scheduled task is not managed by this observer"))
	}
	return state, nil
}

func validTaskXML(actual string, expected []byte) bool {
	owner, err := currentWindowsOwnerSID()
	if err != nil {
		return false
	}
	return validTaskXMLForOwner(actual, expected, owner)
}

func validTaskXMLForOwner(actual string, expected []byte, owner windowsOwnerIdentity) bool {
	want, err := canonicalTaskXMLForOwner(expected, owner)
	if err != nil {
		return false
	}
	got, err := canonicalTaskXMLForOwner([]byte(actual), owner)
	return err == nil && strings.Join(got, "\x00") == strings.Join(want, "\x00")
}

func canonicalTaskXML(value []byte) ([]string, error) {
	owner, err := currentWindowsOwnerSID()
	if err != nil {
		return nil, err
	}
	return canonicalTaskXMLForOwner(value, owner)
}

func canonicalTaskXMLForOwner(value []byte, owner windowsOwnerIdentity) ([]string, error) {
	normalized, err := normalizeTaskXMLBytes(value)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(bytes.NewReader(normalized))
	var tokens []string
	var frames []taskElementFrame
	registrationURISeen := false
	triggerOwnerSeen := false
	defaultFieldsSeen := make(map[string]bool)
	rootSeen := false
	elementCount := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(frames) != 0 {
					return nil, errors.New("task XML element nesting is incomplete")
				}
				if !rootSeen {
					return nil, errors.New("task XML root is missing")
				}
				return tokens, nil
			}
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			elementCount++
			if elementCount > maxTaskXMLElements || len(frames)+1 > maxTaskXMLDepth {
				return nil, errors.New("task XML structure exceeds its limit")
			}
			if len(frames) == 0 {
				if rootSeen || len(tokens) != 0 {
					return nil, errors.New("task XML contains multiple roots")
				}
				if typed.Name.Space != taskXMLNamespace || typed.Name.Local != "Task" {
					return nil, errors.New("task XML root is not Task")
				}
				rootSeen = true
			}
			parents := taskFrameNames(frames)
			path, pathOK := exactTaskPath(parents, typed.Name)
			childStart := -1
			if len(frames) != 0 && frames[len(frames)-1].allGroup {
				parent := &frames[len(frames)-1]
				if typed.Name.Space != taskXMLNamespace || !allowedTaskAllChild(parent.path, typed.Name.Local) {
					return nil, errors.New("task XML contains an unknown child in an unordered group")
				}
				parent.counts[typed.Name.Local]++
				if parent.counts[typed.Name.Local] != 1 {
					return nil, errors.New("task XML contains a duplicate child in an unordered group")
				}
				childStart = len(tokens)
			}
			if isTaskRegistrationURI(parents, typed.Name) {
				if registrationURISeen {
					return nil, errors.New("task XML contains duplicate registration URI metadata")
				}
				if err := consumeExactSimpleTaskElement(decoder, typed, managerIdentity); err != nil {
					return nil, errors.New("task XML registration URI does not match the managed task")
				}
				registrationURISeen = true
				continue
			}
			if isTaskTriggerOwner(parents, typed.Name) {
				if triggerOwnerSeen {
					return nil, errors.New("task XML contains duplicate logon trigger owner")
				}
				if err := consumeTaskTriggerOwner(decoder, typed, owner); err != nil {
					return nil, err
				}
				tokens = append(tokens,
					"<"+typed.Name.Space+"|"+typed.Name.Local+" >",
					"=<current-owner>",
					"</"+typed.Name.Space+"|"+typed.Name.Local+">",
				)
				triggerOwnerSeen = true
				continue
			}
			if defaultPath, defaultValue, ok := normalizedTaskDefault(parents, typed.Name); ok {
				if defaultFieldsSeen[defaultPath] {
					return nil, errors.New("task XML contains duplicate default-valued metadata")
				}
				if err := consumeExactSimpleTaskElement(decoder, typed, defaultValue); err != nil {
					return nil, errors.New("task XML contains changed default-valued metadata")
				}
				defaultFieldsSeen[defaultPath] = true
				continue
			}
			attributes := make([]string, 0, len(typed.Attr))
			for _, attr := range typed.Attr {
				attributes = append(attributes, attr.Name.Space+"|"+attr.Name.Local+"="+attr.Value)
			}
			sort.Strings(attributes)
			tokens = append(tokens, "<"+typed.Name.Space+"|"+typed.Name.Local+" "+strings.Join(attributes, " ")+">")
			allGroup := pathOK && isTaskAllGroup(path)
			frames = append(frames, taskElementFrame{
				name: typed.Name, path: path, start: len(tokens) - 1, childStart: childStart,
				allGroup: allGroup, counts: make(map[string]int),
			})
		case xml.EndElement:
			if len(frames) == 0 || frames[len(frames)-1].name != typed.Name {
				return nil, errors.New("task XML element nesting is invalid")
			}
			tokens = append(tokens, "</"+typed.Name.Space+"|"+typed.Name.Local+">")
			frame := frames[len(frames)-1]
			if frame.allGroup {
				if err := canonicalizeTaskAllGroup(tokens, frame); err != nil {
					return nil, err
				}
			}
			frames = frames[:len(frames)-1]
			if len(frames) != 0 && frame.childStart >= 0 {
				parent := &frames[len(frames)-1]
				parent.children = append(parent.children, taskChildSpan{name: frame.name.Local, start: frame.childStart, end: len(tokens)})
			}
		case xml.CharData:
			if text := strings.TrimSpace(string(typed)); text != "" {
				if len(frames) != 0 && frames[len(frames)-1].allGroup {
					return nil, errors.New("task XML contains text in an unordered group")
				}
				tokens = append(tokens, "="+text)
			}
		}
	}
}

func taskFrameNames(frames []taskElementFrame) []xml.Name {
	names := make([]xml.Name, len(frames))
	for index := range frames {
		names[index] = frames[index].name
	}
	return names
}

// These are the exact xs:all groups used by the observer's fixed task profile.
// Other Task Scheduler structures remain order-sensitive and exact.
func isTaskAllGroup(path string) bool {
	switch path {
	case "Task", "Task/RegistrationInfo", "Task/Settings", "Task/Settings/IdleSettings",
		"Task/Settings/RestartOnFailure", "Task/Principals/Principal", "Task/Actions/Exec":
		return true
	default:
		return false
	}
}

func allowedTaskAllChild(path, child string) bool {
	var allowed string
	switch path {
	case "Task":
		allowed = " RegistrationInfo Triggers Settings Data Principals Actions "
	case "Task/RegistrationInfo":
		allowed = " Description URI "
	case "Task/Settings":
		allowed = " MultipleInstancesPolicy DisallowStartIfOnBatteries StopIfGoingOnBatteries AllowHardTerminate StartWhenAvailable RunOnlyIfNetworkAvailable IdleSettings AllowStartOnDemand Enabled Hidden RunOnlyIfIdle DisallowStartOnRemoteAppSession UseUnifiedSchedulingEngine WakeToRun ExecutionTimeLimit Priority RestartOnFailure "
	case "Task/Settings/IdleSettings":
		allowed = " StopOnIdleEnd RestartOnIdle "
	case "Task/Settings/RestartOnFailure":
		allowed = " Interval Count "
	case "Task/Principals/Principal":
		allowed = " UserId LogonType RunLevel "
	case "Task/Actions/Exec":
		allowed = " Command Arguments WorkingDirectory "
	default:
		return false
	}
	return strings.Contains(allowed, " "+child+" ")
}

func requiredTaskAllChildren(path string) []string {
	switch path {
	case "Task":
		return []string{"Actions"}
	case "Task/Settings/RestartOnFailure":
		return []string{"Interval", "Count"}
	case "Task/Actions/Exec":
		return []string{"Command"}
	default:
		return nil
	}
}

// Task Scheduler may persist an xs:all group's children in any order. Sort
// only the exact groups above, preserving every child subtree byte-for-token.
func canonicalizeTaskAllGroup(tokens []string, frame taskElementFrame) error {
	for _, required := range requiredTaskAllChildren(frame.path) {
		if frame.counts[required] != 1 {
			return errors.New("task XML is missing a required child in an unordered group")
		}
	}
	cursor := frame.start + 1
	for _, child := range frame.children {
		if child.start != cursor || child.end <= child.start || child.end > len(tokens)-1 {
			return errors.New("task XML unordered child boundaries are invalid")
		}
		cursor = child.end
	}
	if cursor != len(tokens)-1 {
		return errors.New("task XML contains content outside unordered children")
	}
	sort.Slice(frame.children, func(left, right int) bool { return frame.children[left].name < frame.children[right].name })
	inner := make([]string, 0, len(tokens)-frame.start-2)
	for _, child := range frame.children {
		inner = append(inner, tokens[child.start:child.end]...)
	}
	copy(tokens[frame.start+1:len(tokens)-1], inner)
	return nil
}

func currentWindowsOwnerSID() (windowsOwnerIdentity, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return windowsOwnerIdentity{}, err
	}
	sid := tokenUser.User.Sid.String()
	if sid == "" {
		return windowsOwnerIdentity{}, errors.New("current Windows owner SID is empty")
	}
	return windowsOwnerIdentity{sid: sid}, nil
}

func (a *windowsAdapter) currentWindowsOwnerIdentity(ctx context.Context) (windowsOwnerIdentity, error) {
	owner, err := currentWindowsOwnerSID()
	if err != nil {
		return windowsOwnerIdentity{}, err
	}
	if a.runner == nil {
		return owner, nil
	}
	const resolveCurrentSID = `& { param($ownerSid) $ErrorActionPreference='Stop'; $sid=New-Object Security.Principal.SecurityIdentifier($ownerSid); $name=$sid.Translate([Security.Principal.NTAccount]).Value; $bytes=[Text.Encoding]::UTF8.GetBytes($name); [Console]::Out.Write([Convert]::ToBase64String($bytes)) }`
	result, err := a.runner.run(ctx, command{
		name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", resolveCurrentSID, owner.sid}, capture: true,
	})
	if err != nil {
		return windowsOwnerIdentity{}, err
	}
	encoded := result.output
	if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxOwnerNameBytes) {
		return windowsOwnerIdentity{}, errors.New("current Windows owner name is invalid")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > maxOwnerNameBytes || !utf8.Valid(decoded) {
		return windowsOwnerIdentity{}, errors.New("current Windows owner name is invalid")
	}
	qualified := string(decoded)
	domain, account, ok := strings.Cut(qualified, `\`)
	if !ok || domain == "" || account == "" || strings.Contains(account, `\`) || containsControl(qualified) {
		return windowsOwnerIdentity{}, errors.New("current Windows owner name is invalid")
	}
	owner.qualifiedSAM = qualified
	computer, err := windows.ComputerName()
	if err == nil && windowsOrdinalEqualFold(domain, computer) {
		owner.localSAM = account
	}
	return owner, nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func windowsOrdinalEqualFold(left, right string) bool {
	if left == right {
		return true
	}
	leftUTF16, err := windows.UTF16FromString(left)
	if err != nil {
		return false
	}
	rightUTF16, err := windows.UTF16FromString(right)
	if err != nil {
		return false
	}
	result, _, _ := compareStringOrdinal.Call(
		uintptr(unsafe.Pointer(&leftUTF16[0])), uintptr(len(leftUTF16)-1),
		uintptr(unsafe.Pointer(&rightUTF16[0])), uintptr(len(rightUTF16)-1),
		1,
	)
	return result == 2
}

func (owner windowsOwnerIdentity) matches(value string) bool {
	if owner.sid != "" && value == owner.sid {
		return true
	}
	return owner.qualifiedSAM != "" && windowsOrdinalEqualFold(value, owner.qualifiedSAM) ||
		owner.localSAM != "" && windowsOrdinalEqualFold(value, owner.localSAM)
}

func isTaskTriggerOwner(parents []xml.Name, current xml.Name) bool {
	path, ok := exactTaskPath(parents, current)
	return ok && path == "Task/Triggers/LogonTrigger/UserId"
}

func consumeTaskTriggerOwner(decoder *xml.Decoder, start xml.StartElement, owner windowsOwnerIdentity) error {
	if len(start.Attr) != 0 {
		return errors.New("task XML logon trigger owner contains attributes")
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
			if typed.Name != start.Name || !owner.matches(contents.String()) {
				return errors.New("task XML logon trigger owner changed")
			}
			return nil
		default:
			return errors.New("task XML logon trigger owner is not a simple value")
		}
	}
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
