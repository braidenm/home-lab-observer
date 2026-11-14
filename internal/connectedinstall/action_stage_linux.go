//go:build linux

package connectedinstall

import (
	"context"
	"os"
	"path/filepath"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
)

const actionReleaseRoot = "/opt/home-lab-observer-actions"

// StageActionRelease only stages verified bytes. It does not provision,
// enroll, register or start a service. Existing state is never adopted.
func StageActionRelease(ctx context.Context, source, digest string, identity connectedidentity.Identity) error {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	if verifyActionStageSource(source, digest, identity) != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	if preflight(ctx) != nil {
		return ErrUnsafe
	}
	for _, path := range []string{actionReleaseRoot, "/etc/home-lab-observer-actions", "/var/lib/home-lab-observer-actions", "/run/home-lab-observer-actions",
		"/etc/systemd/system/home-lab-observer-action-worker.service", "/etc/systemd/system/home-lab-observer-action-broker.service"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	release := filepath.Join(actionReleaseRoot, "releases", digest)
	for _, path := range []string{actionReleaseRoot, filepath.Dir(release), release} {
		if ctx.Err() != nil {
			return ErrRecovery
		}
		parent, err := rootDirectory(filepath.Dir(path))
		if err != nil {
			return ErrRecovery
		}
		createErr := createDirectoryAt(parent, directorySpec{name: filepath.Base(path), mode: 0755}, nil)
		closeErr := parent.Close()
		if createErr != nil || closeErr != nil {
			return ErrRecovery
		}
	}
	if err := copyBundleFiles(source, release, digest, connectedbundle.ActionNames(), connectedbundle.VerifyActionDirectory); err != nil || ctx.Err() != nil {
		return ErrRecovery
	}
	return nil
}

func verifyActionStageSource(source, digest string, identity connectedidentity.Identity) error {
	if identity.Validate() != nil || identity.Role != "action-setup" {
		return ErrUnsafe
	}
	m, err := connectedbundle.VerifyActionDirectory(source, digest)
	if err != nil || m.Version != identity.Version || m.Commit != identity.Commit ||
		m.ContractSHA256 != identity.ContractSHA256 || m.OS != identity.OS || m.Arch != identity.Arch {
		return ErrUnsafe
	}
	return nil
}
