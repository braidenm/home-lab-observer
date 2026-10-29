//go:build linux

package connectedinstall

import (
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

// This private closed choice reuses audited machinery, never caller-provided
// principals, paths, commands or sandbox plans. Zero preserves numeric callers.
type workerPair uint8

const (
	numericPair workerPair = iota
	richProcessesPair
	richProcessesDockerPair
)

func (p workerPair) principalNames() (collector, uploader, shared string, ok bool) {
	switch p {
	case numericPair:
		return collectorName, uploaderName, sharedName, true
	case richProcessesPair:
		return connectedprofile.RichProcessesCollectorName, connectedprofile.RichProcessesUploaderName, connectedprofile.RichProcessesSharedName, true
	case richProcessesDockerPair:
		return connectedprofile.RichProcessesDockerCollectorName, connectedprofile.RichProcessesDockerUploaderName, connectedprofile.RichProcessesDockerSharedName, true
	default:
		return "", "", "", false
	}
}

func (p workerPair) units() (collector, uploader, enrollment string, ok bool) {
	switch p {
	case numericPair:
		return collectorUnit, uploaderUnit, enrollmentUnit, true
	case richProcessesPair:
		return connectedunits.RichProcessesCollectorUnit, connectedunits.RichProcessesUploaderUnit, "home-lab-observer-connected-rich-enrollment.service", true
	case richProcessesDockerPair:
		return connectedunits.RichProcessesDockerCollectorUnit, connectedunits.RichProcessesDockerUploaderUnit, "home-lab-observer-connected-rich-docker-enrollment.service", true
	default:
		return "", "", "", false
	}
}

func (p workerPair) uploaderExecutable(artifact string) string {
	if p == numericPair {
		return connectedprofile.ReleaseDirectory + "/" + artifact + "/observer-connected-uploader"
	}
	if p == richProcessesPair {
		return connectedprofile.RichProcessesReleaseDirectory + "/" + artifact + "/observer-connected-rich-uploader"
	}
	if p == richProcessesDockerPair {
		return connectedprofile.RichProcessesDockerReleaseDirectory + "/" + artifact + "/observer-connected-rich-docker-uploader"
	}
	return ""
}

func knownInstalledUnit(unit string, enrollment bool) bool {
	if unit == connectedunits.RichProcessesDockerReaderUnit {
		return true
	}
	for _, pair := range []workerPair{numericPair, richProcessesPair, richProcessesDockerPair} {
		collector, uploader, transient, _ := pair.units()
		if unit == collector || unit == uploader || enrollment && unit == transient {
			return true
		}
	}
	return false
}

func (p workerPair) stateRoot() string {
	if p == numericPair {
		return connectedprofile.StateDirectory
	}
	if p == richProcessesPair || p == richProcessesDockerPair {
		return connectedprofile.RichProcessesStateDirectory
	}
	return ""
}

func (p workerPair) activationPaths() (parent, path string, ok bool) {
	if p == numericPair {
		return activationParent, activationPath, true
	}
	if p == richProcessesPair || p == richProcessesDockerPair {
		return "/run/home-lab-observer-connected-rich", "/run/home-lab-observer-connected-rich/activation", true
	}
	return "", "", false
}
