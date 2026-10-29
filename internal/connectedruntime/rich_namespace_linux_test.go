//go:build linux

package connectedruntime

import (
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRichNamespaceRequiresPolicyAndExactSevenBindings(t *testing.T) {
	root := "/fixed-rich-root"
	targets := []string{"/bin/observer-connected-rich-uploader", "/etc/home-lab-observer-connected-rich/installed.json", "/etc/home-lab-observer-connected-rich/collection-policy.json", "/handoff", "/activation", "/state/ledger", "/state/status"}
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
		value, ok := objects[name]
		if !ok {
			return value, ErrUnsafe
		}
		return value, nil
	}
	physical := func(path string) (unix.Stat_t, error) {
		if path == root {
			return objects["."], nil
		}
		return actual(strings.TrimPrefix(path, root+"/"))
	}
	if checkNamespaceBindingsForProfile(root, sources, true, actual, physical) != nil {
		t.Fatal("exact rich bindings refused")
	}
	if checkNamespaceBindings(root, sources, actual, physical) != ErrUnsafe {
		t.Fatal("numeric namespace accepted rich mount profile")
	}
	policy := sources[targets[2]]
	delete(sources, targets[2])
	if checkNamespaceBindingsForProfile(root, sources, true, actual, physical) != ErrUnsafe {
		t.Fatal("missing immutable policy bind accepted")
	}
	sources[targets[2]] = policy
	for _, target := range targets {
		name := strings.TrimPrefix(target, "/")
		if checkNamespaceBindingsForProfile(root, sources, true, func(path string) (unix.Stat_t, error) {
			value, err := actual(path)
			if path == name {
				value.Ino++
			}
			return value, err
		}, physical) != ErrUnsafe {
			t.Fatal("substituted rich mount accepted")
		}
	}
}
