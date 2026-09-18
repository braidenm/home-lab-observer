//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

// installedLedgerIdentity binds the named private D1 objects through anchored,
// no-follow descriptors. The installer lease and stopped workers are required.
func installedLedgerIdentity(ctx context.Context, c connectedprofile.Config) (ledgeridentity.Witness, error) {
	return ledgerIdentityAt(ctx, connectedprofile.StateDirectory, c.UploaderUID, c.UploaderGID)
}

func ledgerIdentityAt(ctx context.Context, statePath string, uid, gid uint32) (ledgeridentity.Witness, error) {
	if ctx == nil || ctx.Err() != nil || uid == 0 || gid == 0 {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	state, err := rootDirectory(statePath)
	if err != nil {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	defer state.Close()
	fd, err := unix.Openat(int(state.Fd()), "ledger", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	directory := os.NewFile(uintptr(fd), "private-ledger")
	defer directory.Close()
	info, err := directory.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(fd) {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uid || owner.Gid != gid {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	entries, err := directory.ReadDir(4)
	if err != nil && err != io.EOF || len(entries) < 2 || len(entries) > 3 {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	seen := map[string]bool{}
	var auditedDatabase os.FileInfo
	var totalBytes int64
	for _, entry := range entries {
		name, limit := entry.Name(), int64(0)
		switch name {
		case "upload.sqlite":
			limit = 262144
		case ".upload-lock":
		case "upload.sqlite-journal":
			limit = 1048576
		default:
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		if seen[name] {
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		seen[name] = true
		member, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		file := os.NewFile(uintptr(member), "private-ledger-member")
		valid := regular(file, uid, 0600, limit)
		stat, statErr := file.Stat()
		closeErr := file.Close()
		if !valid || statErr != nil || closeErr != nil || stat.Size() < 0 {
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		memberOwner, ok := stat.Sys().(*syscall.Stat_t)
		if !ok || memberOwner.Gid != gid {
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		totalBytes += stat.Size()
		if totalBytes > 1048576 {
			return ledgeridentity.Witness{}, ErrUnsafe
		}
		if name == "upload.sqlite" {
			auditedDatabase = stat
		}
	}
	if !seen["upload.sqlite"] || !seen[".upload-lock"] {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	// Reopen the database after the membership audit and let Inspect detect
	// any replacement between its two descriptor observations.
	dbFD, err := unix.Openat(fd, "upload.sqlite", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	database := os.NewFile(uintptr(dbFD), "private-ledger-database")
	defer database.Close()
	currentDatabase, err := database.Stat()
	if err != nil || !regular(database, uid, 0600, 262144) || !os.SameFile(auditedDatabase, currentDatabase) {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	currentOwner, ok := currentDatabase.Sys().(*syscall.Stat_t)
	if !ok || currentOwner.Gid != gid {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	witness, err := ledgeridentity.Inspect(ctx, directory, database)
	if err != nil {
		return ledgeridentity.Witness{}, ErrUnsafe
	}
	return witness, nil
}
