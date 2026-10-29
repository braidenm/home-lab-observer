//go:build linux

package connectedruntime

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDockerNamespacesRequireTheirOwnClosedBindings(t *testing.T) {
	for _, reader := range []bool{false, true} {
		targets := dockerNamespaceTargets(reader)
		root := "/synthetic-docker-root"
		sources := map[string]string{}
		objects := map[string]unix.Stat_t{".": {Dev: 2, Ino: 1, Mode: unix.S_IFDIR | 0755}}
		for _, spec := range uploaderScaffold[:6] {
			objects[spec.name] = unix.Stat_t{Dev: 2, Ino: uint64(len(objects) + 1), Mode: unix.S_IFDIR | spec.mode}
		}
		for _, target := range targets {
			sources[target] = root + target
			objects[strings.TrimPrefix(target, "/")] = unix.Stat_t{Dev: 2, Ino: uint64(len(objects) + 1), Mode: unix.S_IFREG | 0644}
		}
		actual := func(name string) (unix.Stat_t, error) {
			s, ok := objects[name]
			if !ok {
				return s, ErrUnsafe
			}
			return s, nil
		}
		physical := func(path string) (unix.Stat_t, error) {
			if path == root {
				return objects["."], nil
			}
			return actual(strings.TrimPrefix(path, root+"/"))
		}
		if checkNamespaceBindingsTargets(root, sources, targets, actual, physical) != nil {
			t.Fatal("closed Docker binding refused")
		}
		if checkNamespaceBindingsForProfile(root, sources, true, actual, physical) != ErrUnsafe {
			t.Fatal("old profile adopted Docker mount")
		}
		for _, target := range targets {
			name := strings.TrimPrefix(target, "/")
			if checkNamespaceBindingsTargets(root, sources, targets, func(path string) (unix.Stat_t, error) {
				s, e := actual(path)
				if path == name {
					s.Ino++
				}
				return s, e
			}, physical) != ErrUnsafe {
				t.Fatal("substituted Docker binding accepted")
			}
			original := sources[target]
			delete(sources, target)
			if checkNamespaceBindingsTargets(root, sources, targets, actual, physical) != ErrUnsafe {
				t.Fatal("missing Docker binding accepted")
			}
			sources[target] = original
		}
		sources["/foreign"] = root + "/foreign"
		if checkNamespaceBindingsTargets(root, sources, targets, actual, physical) != ErrUnsafe {
			t.Fatal("extra authority mount accepted")
		}
	}
}

func TestDockerReaderTargetsContainNoUploadAuthority(t *testing.T) {
	want := []string{"/bin/observer-connected-rich-docker-reader", "/etc/home-lab-observer-connected-rich/installed.json", "/etc/home-lab-observer-connected-rich/collection-policy.json", "/activation", "/frame", "/state/status", "/engine/docker.sock"}
	got := dockerNamespaceTargets(true)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatal("reader namespace authority drift")
	}
}
