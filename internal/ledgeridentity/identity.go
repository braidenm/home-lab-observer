// Package ledgeridentity reads private replacement witnesses from anchored handles.
// It does not authenticate ownership, open paths, read data or modify the ledger.
package ledgeridentity

import (
	"encoding/hex"
	"errors"
)

const Version = "observer-ext4-ledger-identity/v1"

var ErrUnavailable = errors.New("ledger_identity_unavailable")

type Object struct {
	Inode      uint64
	Generation uint32
}

// Witness is private comparison data, not a backup/clone detection mechanism.
type Witness struct {
	FilesystemUUID string
	Directory      Object
	Database       Object
}

func Validate(w Witness) error {
	if len(w.FilesystemUUID) != 32 || w.Directory.Inode == 0 || w.Database.Inode == 0 ||
		w.Directory.Generation == 0 || w.Database.Generation == 0 || w.Directory.Inode == w.Database.Inode {
		return ErrUnavailable
	}
	decoded, err := hex.DecodeString(w.FilesystemUUID)
	if err != nil || hex.EncodeToString(decoded) != w.FilesystemUUID || w.FilesystemUUID == "00000000000000000000000000000000" {
		return ErrUnavailable
	}
	return nil
}
