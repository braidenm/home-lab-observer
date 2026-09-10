package background

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLogSettingsAreExplicitAndLegacyCompatible(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		controller, _, _, settings := testController(t)
		if enabled {
			settings.LogSources = []string{"system"}
		}
		if _, err := controller.Enable(context.Background(), settings); err != nil {
			t.Fatal(err)
		}
		loaded, err := controller.LoadSettings()
		if err != nil || !reflect.DeepEqual(loaded, settings) {
			t.Fatal("settings did not round trip")
		}
		encoded, err := os.ReadFile(filepath.Join(controller.backgroundDir, settingsFile))
		if err != nil || bytes.Contains(encoded, []byte("log_sources")) != enabled {
			t.Fatal("legacy profile changed or explicit sources lost")
		}
		if enabled {
			settings.LogSources[0] = "application"
			if loaded.LogSources[0] != "system" {
				t.Fatal("settings retained caller storage")
			}
			settings.LogSources = nil
			if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeRegistrationMismatch) {
				t.Fatal("source configuration changed silently")
			}
		}
	}
}

func TestInvalidLogSettingsDoNotRegister(t *testing.T) {
	for _, sources := range [][]string{{"unknown"}, {"system", "system"}, {"system", "application", "system"}} {
		controller, adapter, _, settings := testController(t)
		settings.LogSources = sources
		if _, err := controller.Enable(context.Background(), settings); !IsCode(err, CodeInvalidSettings) {
			t.Fatal("bad source configuration accepted")
		}
		if adapter.registered != 0 || adapter.started != 0 {
			t.Fatal("invalid settings caused manager mutation")
		}
	}
}
