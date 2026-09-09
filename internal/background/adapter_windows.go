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

	"golang.org/x/sys/windows"
)

const managerIdentity = `\Home Lab Observer`

type windowsAdapter struct {
	runner commandRunner
	root   string
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
	argument := fmt.Sprintf(`-NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "&amp; &apos;%s&apos; background run --install-root &apos;%s&apos;"`, xmlEscape(launcher), xmlEscape(a.root))
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Home Lab Observer per-user session profile</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>%s</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Owner"><UserId>%s</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><AllowHardTerminate>true</AllowHardTerminate><StartWhenAvailable>true</StartWhenAvailable><RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable><IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings><AllowStartOnDemand>true</AllowStartOnDemand><Enabled>true</Enabled><Hidden>false</Hidden><RunOnlyIfIdle>false</RunOnlyIfIdle><WakeToRun>false</WakeToRun><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><RestartOnFailure><Interval>PT1M</Interval><Count>3</Count></RestartOnFailure><Priority>7</Priority></Settings>
  <Actions Context="Owner"><Exec><Command>%s</Command><Arguments>%s</Arguments></Exec></Actions>
</Task>
`, sid, sid, xmlEscape(powershell), argument)
	return registration{fileName: "task.xml", content: []byte(content)}, nil
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
	decoder := xml.NewDecoder(bytes.NewReader(value))
	var tokens []string
	for {
		token, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return tokens, nil
			}
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			attributes := make([]string, 0, len(typed.Attr))
			for _, attr := range typed.Attr {
				attributes = append(attributes, attr.Name.Space+"|"+attr.Name.Local+"="+attr.Value)
			}
			sort.Strings(attributes)
			tokens = append(tokens, "<"+typed.Name.Space+"|"+typed.Name.Local+" "+strings.Join(attributes, " ")+">")
		case xml.EndElement:
			tokens = append(tokens, "</"+typed.Name.Space+"|"+typed.Name.Local+">")
		case xml.CharData:
			if text := strings.TrimSpace(string(typed)); text != "" {
				tokens = append(tokens, "="+text)
			}
		}
	}
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
