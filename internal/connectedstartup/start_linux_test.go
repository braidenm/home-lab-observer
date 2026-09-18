//go:build linux

package connectedstartup

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestAbsentRequestWaitsUntilCancellation(t *testing.T) {
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	data, err := waitRecord(ctx, directory, "request.json", uint32(os.Getgid()))
	if err != ErrUnsafe || data != nil || time.Since(start) > time.Second {
		t.Fatal("absent request escaped deadline")
	}
	entries, err := directory.ReadDir(-1)
	if err != nil || len(entries) != 0 {
		t.Fatal("waiting created authority")
	}
}

func TestSuccessfulOwnedLoopbackIsNotDenial(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if deniedLoopback(ctx, "tcp4", "127.0.0.1", port) {
		t.Fatal("working connection accepted as filter proof")
	}
}

func TestRootRequestReaderRejectsAliasesAndBroadFiles(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit reviewed fresh temporary root fixture only")
	}
	for _, scenario := range []string{"valid", "broad", "symlink", "fifo", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			dir, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			file := filepath.Join(path, "request.json")
			switch scenario {
			case "symlink":
				if err := os.WriteFile(filepath.Join(path, "target"), []byte("{}"), 0640); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("target", file); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(file, 0640); err != nil {
					t.Fatal(err)
				}
			default:
				data := []byte("{}")
				if scenario == "oversize" {
					data = make([]byte, 2049)
				}
				if err := os.WriteFile(file, data, 0640); err != nil {
					t.Fatal(err)
				}
				mode := os.FileMode(0640)
				if scenario == "broad" {
					mode = 0644
				}
				if err := os.Chmod(file, mode); err != nil {
					t.Fatal(err)
				}
			}
			data, err := readRecord(dir, "request.json", uint32(os.Getgid()))
			if scenario == "valid" {
				if err != nil || string(data) != "{}" {
					t.Fatal("valid root record refused", err)
				}
			} else if err != ErrUnsafe {
				t.Fatal("unsafe record accepted")
			}
		})
	}
}
