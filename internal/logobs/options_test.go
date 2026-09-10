package logobs

import (
	"reflect"
	"testing"
)

func TestExplicitSourceConfiguration(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		got, err := ParseSources(goos, nil)
		if err != nil || got != nil {
			t.Fatal("default must remain disabled")
		}
		got, err = ParseSources(goos, []string{"system"})
		if err != nil || !reflect.DeepEqual(got, []Source{SourceSystem}) {
			t.Fatal("system option rejected")
		}
		for _, bad := range [][]string{{""}, {"SYSTEM"}, {" system"}, {"system,application"}, {"system", "system"}, {"application", "application"}, {"system", "application", "system"}, {"/private/log"}} {
			if result, err := ParseSources(goos, bad); err == nil || result != nil {
				t.Fatal("invalid options accepted")
			}
		}
		got, err = ParseSources(goos, []string{"application", "system"})
		if goos == "linux" {
			if err == nil || got != nil {
				t.Fatal("Linux application accepted")
			}
		} else if err != nil || !reflect.DeepEqual(got, []Source{SourceSystem, SourceApplication}) {
			t.Fatal("canonical order lost")
		}
	}
}
