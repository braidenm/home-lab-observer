//go:build linux

package connectedinstall

import (
	"os"
	"path/filepath"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

type dockerInstallLayout struct {
	richInstallLayout
	readerUID, frameGID, socketGID uint32
}

const (
	dirDockerFrame directoryRole = 200 + iota
	dirDockerStatus
	dirDockerRoot
	dirDockerBin
	dirDockerEtc
	dirDockerConfig
	dirDockerFrameMount
	dirDockerState
	dirDockerStatusMount
	dirDockerEngine
	dirDockerInertRoot
	dirDockerInertUsr
	dirDockerInertVar
	dirDockerInertProc
	dirDockerInertSys
	dirDockerInertDev
	dirDockerInertMqueue
	dirDockerInertRun
	dirDockerInertSystemd
	dirDockerInertIncoming
	dirDockerActivationMount
)
const fileDockerReaderUnit fileRole = 200

var dockerDirectories = []directoryRole{dirDockerFrame, dirDockerStatus, dirDockerRoot, dirDockerBin, dirDockerEtc, dirDockerConfig, dirDockerFrameMount, dirDockerState, dirDockerStatusMount, dirDockerEngine, dirDockerInertRoot, dirDockerInertUsr, dirDockerInertVar, dirDockerInertProc, dirDockerInertSys, dirDockerInertDev, dirDockerInertMqueue, dirDockerInertRun, dirDockerInertSystemd, dirDockerInertIncoming, dirDockerActivationMount}

func (l dockerInstallLayout) directory(role directoryRole) (directorySpec, error) {
	root := connectedprofile.RichProcessesDockerReaderRoot
	spec := func(parent, name string, mode os.FileMode) directorySpec {
		return directorySpec{parent: parent, name: name, mode: mode}
	}
	switch role {
	case dirDockerFrame, dirDockerStatus:
		if _, err := connectedprofile.NewRichProcessesDockerPolicy("srv_00000000000000000000000000000000", "agent_00000000000000000000000000000000", l.collectorUID, l.uploaderUID, l.uploaderGID, l.sharedGID, l.readerUID, l.frameGID, l.socketGID); err != nil {
			return directorySpec{}, ErrUnsafe
		}
		if role == dirDockerFrame {
			return directorySpec{connectedprofile.RichProcessesDockerStateDirectory, "docker-frame", l.readerUID, l.frameGID, 0750}, nil
		}
		return directorySpec{connectedprofile.RichProcessesDockerStateDirectory, "reader-status", l.readerUID, l.frameGID, 0700}, nil
	case dirDockerRoot:
		return spec(connectedprofile.RichProcessesDockerStateDirectory, "reader-root", 0755), nil
	case dirDockerBin:
		return spec(root, "bin", 0755), nil
	case dirDockerEtc:
		return spec(root, "etc", 0755), nil
	case dirDockerConfig:
		return spec(root+"/etc", "home-lab-observer-connected-rich", 0755), nil
	case dirDockerFrameMount:
		return spec(root, "frame", 0755), nil
	case dirDockerState:
		return spec(root, "state", 0755), nil
	case dirDockerStatusMount:
		return spec(root+"/state", "status", 0755), nil
	case dirDockerEngine:
		return spec(root, "engine", 0755), nil
	case dirDockerActivationMount:
		return spec(root, "activation", 0755), nil
	case dirDockerInertRoot:
		return spec(root, "root", 0750), nil
	case dirDockerInertUsr:
		return spec(root, "usr", 0755), nil
	case dirDockerInertVar:
		return spec(root, "var", 0755), nil
	case dirDockerInertProc:
		return spec(root, "proc", 0555), nil
	case dirDockerInertSys:
		return spec(root, "sys", 0555), nil
	case dirDockerInertDev:
		return spec(root, "dev", 0555), nil
	case dirDockerInertMqueue:
		return spec(root+"/dev", "mqueue", 0755), nil
	case dirDockerInertRun:
		return spec(root, "run", 0555), nil
	case dirDockerInertSystemd:
		return spec(root+"/run", "systemd", 0755), nil
	case dirDockerInertIncoming:
		return spec(root+"/run/systemd", "incoming", 0755), nil
	default:
		return l.richInstallLayout.directory(role)
	}
}
func (l dockerInstallLayout) file(role fileRole) (fileSpec, error) {
	if role == fileDockerReaderUnit {
		return fileSpec{"/etc/systemd/system", connectedunits.RichProcessesDockerReaderUnit, 0644, connectedunits.MaxRenderedBytes}, nil
	}
	s, err := l.richInstallLayout.file(role)
	if err != nil {
		return fileSpec{}, err
	}
	if role == fileCollectorUnit {
		s.name = connectedunits.RichProcessesDockerCollectorUnit
	}
	if role == fileUploaderUnit {
		s.name = connectedunits.RichProcessesDockerUploaderUnit
	}
	return s, nil
}
func (l dockerInstallLayout) path(role directoryRole) (string, error) {
	s, err := l.directory(role)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.parent, s.name), nil
}
func (l dockerInstallLayout) createDirectory(role directoryRole) error {
	s, err := l.directory(role)
	if err != nil {
		return ErrUnsafe
	}
	p, err := rootDirectory(s.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer p.Close()
	return createDirectoryAt(p, s, nil)
}
func (l dockerInstallLayout) publishFile(role fileRole, b []byte) error {
	s, err := l.file(role)
	if err != nil || len(b) == 0 || len(b) > s.limit {
		return ErrUnsafe
	}
	p, err := rootDirectory(s.parent)
	if err != nil {
		return ErrUnsafe
	}
	defer p.Close()
	return publishFileAt(p, s, b, nil)
}
func prepareDockerRootFiles(l dockerInstallLayout, addresses []string) error {
	if prepareRootFiles(l, addresses) != nil {
		return ErrRecovery
	}
	for _, role := range dockerDirectories {
		if l.createDirectory(role) != nil {
			return ErrRecovery
		}
	}
	return nil
}
func dockerLayout(c connectedprofile.RichProcessesDockerConfig) dockerInstallLayout {
	return dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{digest: c.ArtifactSHA256, collectorUID: c.CollectorUID, uploaderUID: c.UploaderUID, uploaderGID: c.UploaderGID, sharedGID: c.SharedGID}}, readerUID: c.ReaderUID, frameGID: c.FrameGID, socketGID: c.SocketGID}
}
