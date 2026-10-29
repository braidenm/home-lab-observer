package connectedbootstrap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func richProcessesManifest(t *testing.T) []byte {
	t.Helper()
	m := connectedbundle.Manifest{
		Schema: connectedbundle.VersionV2, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40),
		OS: "linux", Arch: "amd64", Profile: "ubuntu24.04-systemd255-amd64-rich-processes-canary",
		ContractSHA256: connectedprofile.RichProcessesDefinitionSHA256,
	}
	for _, name := range connectedbundle.RichProcessesNames() {
		mode, size := uint32(0o644), int64(1)
		if _, resource := connectedunits.RichProcessesResources()[name]; !resource {
			mode = 0o755
		}
		m.Files = append(m.Files, connectedbundle.File{Name: name, SHA256: strings.Repeat("b", 64), Size: size, Mode: mode})
	}
	data, err := connectedbundle.EncodeRichProcesses(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRenderRichProcessesUsesOnlyRichPackagePaths(t *testing.T) {
	manifest := richProcessesManifest(t)
	result, err := RenderRichProcesses(manifest, strings.Repeat("c", 64), 1234)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{
		[]byte("home-lab-observer-connected-rich-processes_0.1.0-canary.1_linux_amd64.tar.gz"),
		[]byte("/observer-connected-rich-install"),
		[]byte("/opt/home-lab-observer-connected-rich/releases/"),
	} {
		if !bytes.Contains(result, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, unwanted := range [][]byte{
		[]byte("home-lab-observer-connected_0.1.0-canary.1_linux_amd64.tar.gz"),
		[]byte("/observer-connected-install"),
		[]byte("/opt/home-lab-observer-connected/releases/"),
	} {
		if bytes.Contains(result, unwanted) {
			t.Fatalf("numeric package reference escaped: %q", unwanted)
		}
	}
	if _, err := Render(manifest, strings.Repeat("c", 64), 1234); err != ErrInvalid {
		t.Fatal("numeric renderer accepted rich manifest")
	}
}
