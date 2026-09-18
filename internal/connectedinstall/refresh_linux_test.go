//go:build linux

package connectedinstall

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestRefreshStateRequiresEvidenceAfterInitialGeneration(t *testing.T) {
	directory := t.TempDir()
	journal := filepath.Join(directory, "refresh.json")
	receipt := filepath.Join(directory, "refresh-complete")
	initial := connectedprofile.Config{PolicyGeneration: 1}
	if _, _, err := refreshStateAt(initial, journal, receipt); err != nil {
		t.Fatal("initial install without refresh evidence refused", err)
	}
	refreshed := initial
	refreshed.PolicyGeneration = 2
	if _, _, err := refreshStateAt(refreshed, journal, receipt); err != ErrRecovery {
		t.Fatalf("refreshed install without evidence must require recovery: %v", err)
	}
	if err := os.WriteFile(receipt, []byte("synthetic incomplete receipt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := refreshStateAt(initial, journal, receipt); err != ErrUnsafe {
		t.Fatalf("orphan completion must refuse: %v", err)
	}
}

func TestRefreshChangesOnlyNetworkGeneration(t *testing.T) {
	c := connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
	next := c
	next.PolicyGeneration++
	next.Addresses = []string{"8.8.8.8"}
	r := refreshRecord{"observer-connected-refresh/v1", c, next}
	data, _ := json.Marshal(r)
	if _, err := decodeRefresh(data); err != nil {
		t.Fatal("valid generation refused", err)
	}
	for _, mutate := range []func(*refreshRecord){
		func(r *refreshRecord) { r.Next.ServerID = "srv_" + strings.Repeat("d", 32) }, func(r *refreshRecord) { r.Next.ConnectorID = "agent_" + strings.Repeat("d", 32) }, func(r *refreshRecord) { r.Next.ArtifactSHA256 = strings.Repeat("d", 64) }, func(r *refreshRecord) { r.Next.UploaderUID++ }, func(r *refreshRecord) { r.Next.PolicyGeneration = 1 }, func(r *refreshRecord) { r.Next.Addresses = []string{"127.0.0.1"} },
	} {
		bad := r
		mutate(&bad)
		b, _ := json.Marshal(bad)
		if _, err := decodeRefresh(b); err != ErrUnsafe {
			t.Fatal("expanded refresh authority accepted")
		}
	}
	if bytes.Equal(completion(data), completion(append(append([]byte(nil), data...), '\n'))) {
		t.Fatal("receipt did not bind exact transaction")
	}
}

func TestRootKnownReplacementRefusesDrift(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit root-owned temporary fixture only")
	}
	for _, scenario := range []string{"success", "wrong-old", "symlink", "staging-collision"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			root, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			parent, err := root.Open(".")
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			old := []byte("old synthetic policy")
			next := []byte("new synthetic policy")
			if err := root.WriteFile("policy", old, 0600); err != nil {
				t.Fatal(err)
			}
			expected := old
			switch scenario {
			case "wrong-old":
				expected = []byte("different")
			case "symlink":
				if err := root.Rename("policy", "target"); err != nil {
					t.Fatal(err)
				}
				if err := root.Symlink("target", "policy"); err != nil {
					t.Fatal(err)
				}
			case "staging-collision":
				if err := root.WriteFile(".install-next", []byte("preserved evidence"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = putKnownAt(parent, root, "policy", expected, next, 0600, 1024)
			got, readErr := root.ReadFile("policy")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if scenario == "success" {
				if err != nil || !bytes.Equal(got, next) {
					t.Fatal("exact replacement failed", err)
				}
			} else if err == nil || !bytes.Equal(got, old) {
				t.Fatal("foreign or interrupted state replaced")
			}
		})
	}
}
