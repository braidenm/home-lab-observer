//go:build linux

package connectedinstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedstagefs"
)

func TestNormalOperationsRefuseAnyRecoveryMarker(t *testing.T) {
	if rejectRecoveryMarkersAt(nil) != ErrUnsafe {
		t.Fatal("nil directory admitted")
	}
	for _, name := range []string{
		"uninstalling.json", "uninstalled.json",
		connectedstagefs.PreparationName, connectedstagefs.StageName,
	} {
		for _, kind := range []string{"regular", "dangling-symlink"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				path := t.TempDir()
				parent, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer parent.Close()
				if err := rejectRecoveryMarkersAt(parent); err != nil {
					t.Fatal("empty directory refused", err)
				}
				marker := filepath.Join(path, name)
				if kind == "regular" {
					err = os.WriteFile(marker, []byte("interrupted"), 0600)
				} else {
					err = os.Symlink("missing-target", marker)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := rejectRecoveryMarkersAt(parent); err != ErrRecovery {
					t.Fatal("recovery marker admitted", err)
				}
			})
		}
	}
}
