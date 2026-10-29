package connectedbootstrap

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func TestDockerBootstrapBindsExactNewPackage(t *testing.T) {
	m := connectedbundle.Manifest{Schema: connectedbundle.VersionV2, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", Profile: "ubuntu24.04-systemd255-amd64-rich-processes-docker-canary", ContractSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256}
	for _, name := range connectedbundle.RichProcessesDockerNames() {
		mode := uint32(0644)
		if _, ok := connectedunits.RichProcessesDockerResources()[name]; !ok {
			mode = 0755
		}
		m.Files = append(m.Files, connectedbundle.File{Name: name, SHA256: strings.Repeat("b", 64), Size: 1, Mode: mode})
	}
	b, err := connectedbundle.EncodeRichProcessesDocker(m)
	if err != nil {
		t.Fatal(err)
	}
	out, err := RenderRichProcessesDocker(b, strings.Repeat("c", 64), 1234)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"home-lab-observer-connected-rich-processes-docker_0.1.0-canary.1_linux_amd64.tar.gz", "/observer-connected-rich-docker-install", "/opt/home-lab-observer-connected-rich/releases/"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatal("Docker helper identity missing")
		}
	}
	for _, f := range []func([]byte, string, int64) ([]byte, error){Render, RenderRichProcesses} {
		if _, err := f(b, strings.Repeat("c", 64), 1234); err != ErrInvalid {
			t.Fatal("old helper accepted Docker")
		}
	}
}
