package main

import (
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/background"
)

func TestBackgroundPassesOnlyExplicitLogSources(t *testing.T) {
	manager := &fakeBackgroundManager{}
	factory := func(background.Config) (background.Manager, error) { return manager, nil }
	root, state := filepath.Join(t.TempDir(), "managed"), filepath.Join(t.TempDir(), "state")
	code := runBackgroundWith([]string{"enable", "--install-root", root, "--state-dir", state, "--log-source", "system"}, io.Discard, io.Discard, factory, nil)
	if code != 0 || !reflect.DeepEqual(manager.settings.LogSources, []string{"system"}) {
		t.Fatal("explicit source not passed")
	}
	var got []string
	code = runBackgroundWith([]string{"run", "--install-root", root}, io.Discard, io.Discard, factory, func(args []string, _, _ io.Writer) int { got = args; return 0 })
	if code != 0 || !reflect.DeepEqual(got[len(got)-2:], []string{"--log-source", "system"}) {
		t.Fatal("persisted source not passed to runtime")
	}
}

func TestInvalidLogFlagsDoNotConstructManager(t *testing.T) {
	for _, extra := range [][]string{{"--log-source", "unknown"}, {"--log-source", "system", "--log-source", "system"}, {"--log-source", "system,application"}} {
		args := append([]string{"enable", "--install-root", "unused", "--state-dir", "unused"}, extra...)
		code := runBackgroundWith(args, io.Discard, io.Discard, func(background.Config) (background.Manager, error) {
			t.Fatal("invalid flags reached manager")
			return nil, nil
		}, nil)
		if code != 2 {
			t.Fatal("invalid flags accepted")
		}
	}
}
