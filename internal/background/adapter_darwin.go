//go:build darwin

package background

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const managerIdentity = "com.braidenm.home-lab-observer"

type darwinAdapter struct {
	runner        commandRunner
	target        string
	domain        string
	root          string
	forceUnloaded bool
}

func launcherName() string { return "observer" }

func newPlatformAdapter(runner commandRunner, root string) (platformAdapter, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, coded(CodeManagerUnavailable, err)
	}
	return &darwinAdapter{runner: runner, target: filepath.Join(home, "Library", "LaunchAgents", managerIdentity+".plist"), domain: "gui/" + strconv.Itoa(os.Getuid()), root: root}, nil
}

func (a *darwinAdapter) registration(Settings) (registration, error) {
	launcher := filepath.Join(a.root, "bin", launcherName())
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>background</string><string>run</string><string>--install-root</string><string>%s</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>ThrottleInterval</key><integer>10</integer>
<key>ProcessType</key><string>Background</string>
<key>StandardOutPath</key><string>/dev/null</string>
<key>StandardErrorPath</key><string>/dev/null</string>
</dict></plist>
`, managerIdentity, xmlEscape(launcher), xmlEscape(a.root))
	return registration{fileName: managerIdentity + ".plist", content: []byte(content)}, nil
}

func (a *darwinAdapter) available(ctx context.Context) bool {
	_, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"print", a.domain}})
	return err == nil
}

func (a *darwinAdapter) inspect(ctx context.Context, expected registration) (managerState, error) {
	if !a.available(ctx) {
		return managerState{}, nil
	}
	state := managerState{available: true}
	matched, exists, err := exactExternalFile(a.target, expected.content)
	if err != nil {
		return state, coded(CodeRegistrationMismatch, err)
	}
	if !exists {
		loaded, loadedErr := a.runner.run(ctx, command{name: "launchctl", args: []string{"print", a.domain + "/" + managerIdentity}})
		if loadedErr == nil {
			return state, coded(CodeRegistrationMismatch, errors.New("launchd has a loaded job without the managed plist"))
		}
		if loaded.exitCode == 0 {
			return state, loadedErr
		}
		return state, nil
	}
	if !matched {
		return state, coded(CodeRegistrationMismatch, errors.New("existing LaunchAgent is not managed by this observer"))
	}
	state.registered = true
	result, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"print", a.domain + "/" + managerIdentity}, capture: true})
	if err == nil {
		state.running = strings.Contains(result.output, "state = running")
	} else if result.exitCode == 0 {
		return state, err
	}
	return state, nil
}

func (a *darwinAdapter) register(ctx context.Context, source string, expected registration) error {
	if err := installExternalFile(a.target, source, expected.content); err != nil {
		return err
	}
	_, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"bootstrap", a.domain, a.target}})
	if err == nil {
		a.forceUnloaded = false
	}
	return err
}

func (a *darwinAdapter) start(ctx context.Context) error {
	if _, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"kickstart", a.domain + "/" + managerIdentity}}); err == nil {
		a.forceUnloaded = false
		return nil
	}
	_, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"bootstrap", a.domain, a.target}})
	if err == nil {
		a.forceUnloaded = false
	}
	return err
}

func (a *darwinAdapter) stop(ctx context.Context, force bool, stopper GracefulStopper, settings Settings) error {
	if !force {
		if stopper == nil {
			return errors.New("graceful lifecycle client is unavailable")
		}
		return stopper.RequestStop(ctx, settings.StateDir)
	}
	if force {
		_, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"bootout", a.domain + "/" + managerIdentity}})
		if err == nil {
			a.forceUnloaded = true
		}
		return err
	}
	_, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"kill", "SIGTERM", a.domain + "/" + managerIdentity}})
	return err
}

func (a *darwinAdapter) unregister(ctx context.Context, _ string, expected registration) error {
	matched, exists, err := exactExternalFile(a.target, expected.content)
	if err != nil || (exists && !matched) {
		return errors.New("refusing to remove an unknown LaunchAgent")
	}
	if !a.forceUnloaded {
		if _, err := a.runner.run(ctx, command{name: "launchctl", args: []string{"bootout", a.domain + "/" + managerIdentity}}); err != nil {
			return err
		}
	}
	if exists {
		return os.Remove(a.target)
	}
	return nil
}
