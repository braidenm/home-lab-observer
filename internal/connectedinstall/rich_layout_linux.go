//go:build linux

package connectedinstall

import (
	"path/filepath"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

// Only compiled directory/file roles are translated. No caller supplies roots,
// modes or path substitutions; publication retains the shared descriptor checks.
type richInstallLayout struct{ installLayout }

const (
	richConfigRoot    directoryRole = 100
	richStateRoot     directoryRole = 101
	richPolicyFile    fileRole      = 100
	richPreparingFile fileRole      = 101
)

func richFixedPath(path string) string {
	for _, root := range []string{"/etc/home-lab-observer-connected", "/var/lib/home-lab-observer-connected", "/opt/home-lab-observer-connected"} {
		if path == root {
			return root + "-rich"
		}
		if suffix, ok := strings.CutPrefix(path, root+"/"); ok {
			return root + "-rich/" + suffix
		}
	}
	return path
}

func (l richInstallLayout) directory(role directoryRole) (directorySpec, error) {
	switch role {
	case richConfigRoot:
		return directorySpec{parent: "/etc", name: "home-lab-observer-connected-rich", mode: 0755}, nil
	case richStateRoot:
		return directorySpec{parent: "/var/lib", name: "home-lab-observer-connected-rich", mode: 0755}, nil
	}
	spec, err := l.installLayout.directory(role)
	if err != nil {
		return directorySpec{}, err
	}
	path := richFixedPath(filepath.Join(spec.parent, spec.name))
	spec.parent, spec.name = filepath.Dir(path), filepath.Base(path)
	if role == dirUploaderConfig {
		spec.name = "home-lab-observer-connected-rich"
	}
	return spec, nil
}

func (l richInstallLayout) file(role fileRole) (fileSpec, error) {
	if role == richPreparingFile {
		return fileSpec{connectedprofile.RichProcessesConfigDirectory, "preparing.json", 0600, 1024}, nil
	}
	if role == richPolicyFile {
		return fileSpec{connectedprofile.RichProcessesConfigDirectory, "collection-policy.json", 0644, connectedprofile.RichProcessesMaxPolicyBytes}, nil
	}
	spec, err := l.installLayout.file(role)
	if err != nil {
		return fileSpec{}, err
	}
	spec.parent = richFixedPath(spec.parent)
	switch role {
	case fileCollectorUnit:
		spec.name = connectedunits.RichProcessesCollectorUnit
	case fileUploaderUnit:
		spec.name = connectedunits.RichProcessesUploaderUnit
	}
	return spec, nil
}

func (l richInstallLayout) path(role directoryRole) (string, error) {
	spec, err := l.directory(role)
	if err != nil {
		return "", err
	}
	return filepath.Join(spec.parent, spec.name), nil
}

func (l richInstallLayout) createDirectory(role directoryRole) error {
	spec, err := l.directory(role)
	if err != nil {
		return ErrUnsafe
	}
	parent, err := rootDirectory(spec.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	return createDirectoryAt(parent, spec, nil)
}

func (l richInstallLayout) publishFile(role fileRole, data []byte) error {
	spec, err := l.file(role)
	if err != nil || len(data) == 0 || len(data) > spec.limit {
		return ErrUnsafe
	}
	parent, err := rootDirectory(spec.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	return publishFileAt(parent, spec, data, nil)
}
