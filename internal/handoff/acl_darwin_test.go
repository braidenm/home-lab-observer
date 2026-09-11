//go:build darwin

package handoff

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestExtendedACLNativeOwnedFixtures(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "owned-acl-fixture")
			if directory {
				if os.Mkdir(path, 0o700) != nil {
					t.Fatal("fixture creation failed")
				}
			} else {
				f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatal("fixture creation failed")
				}
				if f.Close() != nil {
					t.Fatal("fixture close failed")
				}
			}
			// Mutations are limited to this empty synthetic fixture, never ancestors.
			fixtureChmod(t, "-N", path)
			file, err := os.Open(path)
			if err != nil {
				t.Fatal("fixture open failed")
			}
			defer file.Close()
			if noExtendedACL(file) != nil {
				t.Fatal("absent ACL rejected")
			}
			grant := "everyone allow read"
			if directory {
				grant = "everyone allow list,search"
			}
			fixtureChmod(t, "+a", grant, path)
			if !errors.Is(noExtendedACL(file), ErrUnsafe) {
				t.Fatal("extended grant accepted")
			}
			fixtureChmod(t, "-N", path)
			if noExtendedACL(file) != nil {
				t.Fatal("removed ACL rejected")
			}
			if file.Close() != nil {
				t.Fatal("fixture close failed")
			}
			if !errors.Is(noExtendedACL(file), ErrUnsafe) {
				t.Fatal("closed handle accepted")
			}
		})
	}
	if !errors.Is(noExtendedACL(nil), ErrUnsafe) {
		t.Fatal("nil handle accepted")
	}
}

func fixtureChmod(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// No shell, no inherited diagnostic output, no host configuration changes.
	if exec.CommandContext(ctx, "/bin/chmod", args...).Run() != nil {
		t.Fatal("owned fixture ACL operation failed")
	}
}

func TestExtendedACLPolicyFailuresAndReleases(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		acl, empty            uintptr
		errno                 int32
		actualSize, emptySize int64
		freeFailure           uintptr
		wantOK                bool
		wantFreed             int
	}{
		{"absent", 0, 100, int32(unix.ENOENT), 0, 44, 0, true, 0},
		{"null no error", 0, 100, 0, 0, 44, 0, false, 0},
		{"denied", 0, 100, int32(unix.EACCES), 0, 44, 0, false, 0},
		{"empty", 200, 100, 0, 44, 44, 0, true, 2},
		{"nonempty", 200, 100, 0, 68, 44, 0, false, 2},
		{"size error", 200, 100, 0, -1, 44, 0, false, 2},
		{"empty size error", 200, 100, 0, 44, -1, 0, false, 2},
		{"zero sizes", 200, 100, 0, 0, 0, 0, false, 2},
		{"init failure", 200, 0, 0, 44, 44, 0, false, 1},
		{"special sentinel", ^uintptr(0), 100, 0, 44, 44, 0, false, 0},
		{"acl free failure", 200, 100, 0, 44, 44, 200, false, 2},
		{"empty free failure", 200, 100, 0, 44, 44, 100, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			freed := map[uintptr]bool{}
			calls := aclCalls{
				getFD: func(fd int32) (uintptr, int32) {
					if fd != 7 {
						t.Fatal("fd boundary incorrect")
					}
					return tc.acl, tc.errno
				},
				init: func(count int32) uintptr {
					if count != 0 {
						t.Fatal("not empty reference")
					}
					return tc.empty
				},
				size: func(acl uintptr) int64 {
					if acl == tc.empty {
						return tc.emptySize
					}
					return tc.actualSize
				},
				free: func(acl uintptr) int32 {
					if freed[acl] {
						t.Fatal("double free")
					}
					freed[acl] = true
					if acl == tc.freeFailure {
						return -1
					}
					return 0
				},
			}
			err := checkExtendedACL(7, calls)
			if (err == nil) != tc.wantOK || (err != nil && !errors.Is(err, ErrUnsafe)) || len(freed) != tc.wantFreed {
				t.Fatal("ACL policy or resource lifetime mismatch")
			}
		})
	}
	if !errors.Is(checkExtendedACL(7, aclCalls{}), ErrUnsafe) {
		t.Fatal("missing query accepted")
	}
}
