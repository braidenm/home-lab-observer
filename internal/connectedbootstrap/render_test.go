package connectedbootstrap

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
)

func fixture(t *testing.T) ([]byte, []member) {
	t.Helper()
	m := connectedbundle.Manifest{Schema: connectedbundle.VersionV2, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", Profile: connectedbundle.Profile, ContractSHA256: connectedcompat.Digest()}
	for _, name := range connectedbundle.Names() {
		mode := uint32(0o644)
		if strings.HasPrefix(name, "observer-connected-") {
			mode = 0o755
		}
		sum := sha256.Sum256([]byte("x"))
		m.Files = append(m.Files, connectedbundle.File{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: 1, Mode: mode})
	}
	encoded, err := connectedbundle.Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	script, err := Render(encoded, strings.Repeat("b", 64), 100)
	if err != nil {
		t.Fatal(err)
	}
	// Read the exact generated public data, not a second Python fixture model.
	const marker = `json.loads(base64.b64decode("`
	index := bytes.Index(script, []byte(marker))
	if index < 0 {
		t.Fatal("missing extraction data")
	}
	data := strings.SplitN(string(script[index+len(marker):]), `"`, 2)[0]
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	var members []member
	if json.Unmarshal(decoded, &members) != nil {
		t.Fatal("invalid generated members")
	}
	return encoded, members
}

func TestRenderClosedDeterministicHelper(t *testing.T) {
	manifest, members := fixture(t)
	first, err := Render(manifest, strings.Repeat("b", 64), 100)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(manifest, strings.Repeat("b", 64), 100)
	if !bytes.Equal(first, second) || len(first) > 64<<10 || len(members) != 8 {
		t.Fatal("unstable helper")
	}
	for _, required := range []string{"exec /usr/bin/python3 -I -", `3<&0`, `"/usr/bin/curl", "-q", "--fail", "--remove-on-error"`, `"--proxy", "", "--location", "--proto", "=https", "--proto-redir", "=https"`, `"--max-redirs", "5", "--connect-timeout", "10", "--max-time", "180"`, `"--max-filesize", "100"`, `tempfile.mkdtemp(prefix="home-lab-observer-download."`, `digest.hexdigest() != "` + strings.Repeat("b", 64), `signal.pidfd_send_signal`, `child.wait()`, `stdin=3, interactive=True`} {
		if !bytes.Contains(first, []byte(required)) {
			t.Fatalf("missing fixed policy %q", required)
		}
	}
	for _, forbidden := range []string{"rm -rf", "extractall", "systemctl start", "systemctl enable", "Authorization:", "hle_", "GH_TOKEN", "latest/", "--insecure"} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("unexpected helper capability %q", forbidden)
		}
	}
	for _, mutation := range []struct {
		manifest []byte
		digest   string
		size     int64
	}{
		{nil, strings.Repeat("b", 64), 100}, {append(manifest, '\n'), strings.Repeat("b", 64), 100},
		{manifest, "bad", 100}, {manifest, strings.Repeat("b", 64), 0}, {manifest, strings.Repeat("b", 64), (256 << 20) + 1},
		{bytes.ReplaceAll(manifest, []byte("0.1.0-canary.1"), []byte("0.01.0-canary.1")), strings.Repeat("b", 64), 100},
		{bytes.ReplaceAll(manifest, []byte("0.1.0-canary.1"), []byte("1.0.0-stable")), strings.Repeat("b", 64), 100},
	} {
		if _, err := Render(mutation.manifest, mutation.digest, mutation.size); err != ErrInvalid {
			t.Fatal("invalid helper input accepted")
		}
	}
	if runtime.GOOS == "linux" {
		cmd := boundedCommand(t, "/bin/bash", "-n")
		cmd.Stdin = bytes.NewReader(first)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("shell syntax: %v %s", err, output)
		}
		cmd = boundedCommand(t, "/bin/bash", "-c", string(first), "--", "srv_"+strings.Repeat("a", 32))
		cmd.Stdin = strings.NewReader("")
		if output, err := cmd.CombinedOutput(); err == nil || string(output) != "CONNECTED_BOOTSTRAP_REFUSED: root, terminal and one server reference required\n" {
			t.Fatalf("non-terminal execution was not refused: %v %q", err, output)
		}
	}
}

func TestExtractorRefusesForeignOrNonemptyOutputAndBrokenGzip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("connected bootstrap supported on Linux only")
	}
	_, members := fixture(t)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(archiveBytes(t, members)); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"directory-mode", "directory-link", "nonempty", "gzip-truncated", "gzip-crc"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(root, "bundle")
			if err := os.Mkdir(output, 0o700); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(root, "archive.gz")
			data := append([]byte(nil), compressed.Bytes()...)
			switch kind {
			case "directory-mode":
				if err := os.Chmod(output, 0o755); err != nil {
					t.Fatal(err)
				}
			case "directory-link":
				link := filepath.Join(root, "link")
				if err := os.Symlink(output, link); err != nil {
					t.Fatal(err)
				}
				output = link
			case "nonempty":
				if err := os.WriteFile(filepath.Join(output, "keep"), []byte("existing"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "gzip-truncated":
				data = data[:len(data)-3]
			case "gzip-crc":
				data[len(data)-8] ^= 1
			}
			if err := os.WriteFile(archive, data, 0o600); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(members)
			result, err := boundedCommand(t, "/usr/bin/python3", "-I", "-c", extractionProgram(encoded), archive, output).CombinedOutput()
			if err == nil || string(result) != "CONNECTED_BOOTSTRAP_EXTRACTION_REFUSED\n" {
				t.Fatalf("unsafe extraction: %v %q", err, result)
			}
			if kind == "nonempty" {
				kept, err := os.ReadFile(filepath.Join(output, "keep"))
				if err != nil || string(kept) != "existing" {
					t.Fatal("changed existing content")
				}
			}
		})
	}
}

func TestExtractorRejectsRepinnedMalformedArchives(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("connected bootstrap supported on Linux only")
	}
	_, members := fixture(t)
	valid := archiveBytes(t, members)
	for _, test := range []struct {
		name   string
		mutate func([]byte) []byte
		good   bool
	}{
		{"valid", func(b []byte) []byte { return b }, true},
		{"duplicate", func(b []byte) []byte { return append(append([]byte(nil), b[:1024]...), b...) }, false},
		{"symlink", headerMutation(156, []byte("2")), false},
		{"hardlink", headerMutation(156, []byte("1")), false},
		{"device", headerMutation(156, []byte("3")), false},
		{"directory", headerMutation(156, []byte("5")), false},
		{"pax", headerMutation(156, []byte("x")), false},
		{"gnu", headerMutation(156, []byte("L")), false},
		{"absolute", headerMutation(0, []byte("/outside")), false},
		{"traversal", headerMutation(0, []byte("../outside")), false},
		{"oversized", headerMutation(124, []byte("77777777777")), false},
		{"wrong-mode", headerMutation(100, []byte("0000777")), false},
		{"wrong-owner", headerMutation(108, []byte("0000001")), false},
		{"checksum", func(b []byte) []byte { b[148] ^= 1; return b }, false},
		{"header-padding", func(b []byte) []byte { b[511] = 1; return b }, false},
		{"body-digest", func(b []byte) []byte { b[512] ^= 1; return b }, false},
		{"body-padding", func(b []byte) []byte { b[513] = 1; return b }, false},
		{"truncated", func(b []byte) []byte { return b[:len(b)-1] }, false},
		{"end-marker", func(b []byte) []byte { b[len(b)-512] = 1; return b }, false},
		{"trailing", func(b []byte) []byte { return append(b, 1) }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			output := filepath.Join(root, "bundle")
			if err := os.Mkdir(output, 0o700); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(root, "archive.tar.gz")
			var compressed bytes.Buffer
			gz := gzip.NewWriter(&compressed)
			if _, err := gz.Write(test.mutate(append([]byte(nil), valid...))); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(archive, compressed.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(members)
			cmd := boundedCommand(t, "/usr/bin/python3", "-I", "-c", extractionProgram(encoded), archive, output)
			result, err := cmd.CombinedOutput()
			if test.good {
				if err != nil {
					t.Fatalf("valid extraction: %v %s", err, result)
				}
				entries, _ := os.ReadDir(output)
				if len(entries) != len(members) {
					t.Fatal("incomplete extraction")
				}
				for _, member := range members {
					info, err := os.Lstat(filepath.Join(output, member.Name))
					if err != nil || info.Mode().Perm() != os.FileMode(member.Mode) || info.Size() != member.Size {
						t.Fatal("incorrect output metadata")
					}
				}
			} else if err == nil || string(result) != "CONNECTED_BOOTSTRAP_EXTRACTION_REFUSED\n" {
				t.Fatalf("unsafe extraction result: %v %q", err, result)
			}
		})
	}
}

func boundedCommand(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = time.Second
	return command
}

func TestCoordinatorJoinsChildrenAndPreservesTerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("connected bootstrap supported on Linux only")
	}
	fixture, err := os.ReadFile("lifecycle_fixture.py")
	if err != nil {
		t.Fatal(err)
	}
	source, _, found := strings.Cut(coordinatorSource, "\nif __name__ == \"__main__\":")
	if !found {
		t.Fatal("missing closed entry point")
	}
	program := extractionSource + "\n" + source
	encoded := base64.StdEncoding.EncodeToString([]byte(program))
	cmd := boundedCommand(t, "/usr/bin/python3", "-I", "-c", "SOURCE = __import__('base64').b64decode('"+encoded+"').decode()\n"+string(fixture))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lifecycle fixtures: %v\n%s", err, output)
	}
}

func headerMutation(offset int, replacement []byte) func([]byte) []byte {
	return func(b []byte) []byte {
		copy(b[offset:], replacement)
		for i := 148; i < 156; i++ {
			b[i] = ' '
		}
		var sum int64
		for _, v := range b[:512] {
			sum += int64(v)
		}
		// A real alternate tar header checksum prevents treating a generic parse
		// failure as proof that our narrower semantic contract was enforced.
		checksum := []byte(strings.Repeat("0", 6))
		for i := len(checksum) - 1; i >= 0; i-- {
			checksum[i] += byte(sum & 7)
			sum >>= 3
		}
		copy(b[148:154], checksum)
		b[154] = 0
		b[155] = ' '
		return b
	}
}

func archiveBytes(t *testing.T, members []member) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	for _, member := range members {
		var body []byte
		if member.Name == connectedbundle.ManifestName {
			body, _ = fixture(t)
		} else {
			body = []byte("x")
		}
		if err := writer.WriteHeader(&tar.Header{Name: member.Name, Mode: int64(member.Mode), Size: member.Size, Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
