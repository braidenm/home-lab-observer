//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/ledgerwitness"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type refreshRecord struct {
	Version  string                  `json:"version"`
	Previous connectedprofile.Config `json:"previous"`
	Next     connectedprofile.Config `json:"next"`
}

func decodeRefresh(data []byte) (refreshRecord, error) {
	var r refreshRecord
	if len(data) == 0 || len(data) > 8192 || json.Unmarshal(data, &r) != nil {
		return refreshRecord{}, ErrUnsafe
	}
	// The same pure admission now governs existing receipts and newly
	// constructed legacy journals during the staged transition migration.
	if _, err := connectedtransition.AdmitCompletedLegacy(data, completion(data), r.Next); err != nil {
		return refreshRecord{}, ErrUnsafe
	}
	return r, nil
}

func completion(data []byte) []byte {
	digest := sha256.Sum256(data)
	return []byte("observer-connected-refresh-complete/v1\n" + hex.EncodeToString(digest[:]) + "\n")
}

// refreshState returns the last closed journal only when completion and current
// installed identity agree. Missing evidence is valid only for the initial
// generation, before any refresh could have completed.
func refreshState(c connectedprofile.Config) ([]byte, []byte, error) {
	return refreshStateAt(c, connectedprofile.ConfigDirectory+"/refresh.json", connectedprofile.ConfigDirectory+"/refresh-complete")
}

func refreshStateAt(c connectedprofile.Config, journal, receipt string) ([]byte, []byte, error) {
	if _, err := os.Lstat(journal); os.IsNotExist(err) {
		if _, err := os.Lstat(receipt); os.IsNotExist(err) {
			// Only the initial installed generation has no refresh evidence.
			// Losing both files after a refresh must not reset the history.
			if c.PolicyGeneration != 1 {
				return nil, nil, ErrRecovery
			}
			return nil, nil, nil
		}
		return nil, nil, ErrUnsafe
	} else if err != nil {
		return nil, nil, ErrUnsafe
	}
	data, err := connectedprofile.ReadRootFile(journal, 8192)
	if err != nil {
		return nil, nil, ErrRecovery
	}
	got, err := connectedprofile.ReadRootFile(receipt, 256)
	if err != nil {
		return nil, nil, ErrRecovery
	}
	if _, err := connectedtransition.AdmitCompletedLegacy(data, got, c); err != nil {
		return nil, nil, ErrRecovery
	}
	return data, got, nil
}

func hostsBytes(addresses []string) []byte {
	var b strings.Builder
	for _, address := range addresses {
		b.WriteString(address + " " + connectedprofile.Hostname + "\n")
	}
	return []byte(b.String())
}

// Read the existing D1 state through the exact packaged uploader's confined
// offline mode. The result is private comparison data, never an operator status.
func existingLedgerFingerprint(ctx context.Context, c connectedprofile.Config) ([32]byte, error) {
	var zero [32]byte
	policy := connectedunits.EnrollmentInput{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, Addresses: c.Addresses}
	invocation, err := connectedunits.RenderEnrollmentProperties(policy, "validate-existing-ledger")
	if err != nil {
		return zero, ErrUnsafe
	}
	input, err := json.Marshal(connectedenroll.ValidationInput{ServerID: c.ServerID, ConnectorID: c.ConnectorID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID})
	if err != nil {
		return zero, ErrUnsafe
	}
	result, err := runEnrollment(ctx, invocation.Properties, invocation.Executable, "validate-existing-ledger", input, policy)
	defer clear(result)
	if err != nil {
		return zero, ErrRecovery
	}
	fingerprint, err := ledgerwitness.DecodeResult(result, uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID})
	if err != nil {
		return zero, ErrRecovery
	}
	return fingerprint, nil
}

// Refresh validates new fixed-host TLS endpoints before any mutation. It leaves
// both services stopped and disabled; activation must separately pass the exact
// packaged readiness gate. No failure restores old ledger/credentials or starts
// a worker against a mixed generation. A bounded journal preserves recovery facts.
func Refresh(ctx context.Context) error {
	if ctx == nil {
		return ErrUnsafe
	}
	lock, err := acquireLease()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := rejectRecoveryMarkers(); err != nil {
		return err
	}
	c, err := inspectInstalled()
	if err != nil {
		return err
	}
	oldJournal, oldReceipt, err := refreshState(c)
	if err != nil {
		return err
	}
	if c.PolicyGeneration == math.MaxUint64 {
		return ErrUnsafe
	}
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil {
		return ErrUnsafe
	}
	if connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	ca, err := connectedprofile.ReadRootFile("/etc/ssl/certs/ca-certificates.crt", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	next := c
	next.PolicyGeneration++
	next.Addresses = addresses
	nextUnits, err := connectedunits.RenderUnits(next)
	if err != nil {
		return ErrUnsafe
	}
	oldUnits, err := connectedunits.RenderUnits(c)
	if err != nil {
		return ErrUnsafe
	}
	base := connectedprofile.StateDirectory + "/uploader-root"
	oldHosts, err := connectedprofile.ReadRootFile(base+"/etc/hosts", 4096)
	if err != nil || !bytes.Equal(oldHosts, hostsBytes(c.Addresses)) {
		return ErrUnsafe
	}
	oldCA, err := connectedprofile.ReadRootFile(base+"/etc/ssl/certs/ca-certificates.crt", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	oldConfig, _ := connectedprofile.Encode(c)
	newConfig, _ := connectedprofile.Encode(next)
	journal, _ := json.Marshal(refreshRecord{"observer-connected-refresh/v1", c, next})
	if _, err := decodeRefresh(journal); err != nil {
		return ErrUnsafe
	}
	if err := stopOwnedWorkers(ctx); err != nil {
		return ErrRecovery
	}
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		if service(ctx, "disable", unit) != nil {
			return ErrRecovery
		}
	}
	beforeLedger, err := existingLedgerFingerprint(ctx, c)
	if err != nil {
		return err
	}
	beforeIdentity, err := installedLedgerIdentity(ctx, c)
	if err != nil {
		return ErrRecovery
	}
	// The journal is the forward-only publication boundary. Do not publish it
	// while either owned worker might still be running: a failed stop must leave
	// the completed predecessor as the only authoritative configuration.
	if err := putKnown(connectedprofile.ConfigDirectory+"/refresh.json", oldJournal, journal, 0600, 8192); err != nil {
		return err
	}
	for _, item := range []struct {
		path      string
		old, next []byte
		mode      os.FileMode
		limit     int
	}{
		{base + "/etc/hosts", oldHosts, hostsBytes(addresses), 0644, 4096},
		{base + "/etc/ssl/certs/ca-certificates.crt", oldCA, ca, 0644, 1 << 20},
		{"/etc/systemd/system/" + collectorUnit, oldUnits.Collector, nextUnits.Collector, 0644, connectedunits.MaxRenderedBytes},
		{"/etc/systemd/system/" + uploaderUnit, oldUnits.Uploader, nextUnits.Uploader, 0644, connectedunits.MaxRenderedBytes},
		{connectedprofile.ConfigPath, oldConfig, newConfig, 0644, connectedprofile.MaxConfigBytes},
	} {
		if err := putKnown(item.path, item.old, item.next, item.mode, item.limit); err != nil {
			return ErrRecovery
		}
	}
	if _, err := command(ctx, "/usr/bin/systemctl", []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if connectedpolicy.ValidateEffective(ctx, uploaderUnit, addresses) != nil {
		return ErrRecovery
	}
	afterLedger, err := existingLedgerFingerprint(ctx, next)
	if err != nil || afterLedger != beforeLedger {
		return ErrRecovery
	}
	afterIdentity, err := installedLedgerIdentity(ctx, next)
	if err != nil || afterIdentity != beforeIdentity {
		return ErrRecovery
	}
	if err := putKnown(connectedprofile.ConfigDirectory+"/refresh-complete", oldReceipt, completion(journal), 0600, 256); err != nil {
		return ErrRecovery
	}
	return nil
}

// putKnown replaces only an exact previously validated root-owned regular file,
// or publishes a new name when no prior bytes exist. Caller holds installation
// lease throughout. Interrupted staging is preserved rather than auto-repaired.
func putKnown(path string, previous, next []byte, mode os.FileMode, limit int) error {
	if len(next) == 0 || len(next) > limit {
		return ErrUnsafe
	}
	parent, err := rootDirectory(filepath.Dir(path))
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return ErrUnsafe
	}
	defer root.Close()
	name := filepath.Base(path)
	return putKnownAt(parent, root, name, previous, next, mode, limit)
}

func putKnownAt(parent *os.File, root *os.Root, name string, previous, next []byte, mode os.FileMode, limit int) error {
	if len(next) == 0 || len(next) > limit {
		return ErrUnsafe
	}
	if previous == nil {
		return publishBounded(root, name, next, mode, limit)
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	old := os.NewFile(uintptr(fd), name)
	defer old.Close()
	if !regular(old, 0, mode, int64(limit)) {
		return ErrUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(old, int64(limit)+1))
	if err != nil || !bytes.Equal(data, previous) {
		return ErrUnsafe
	}
	before, err := old.Stat()
	if err != nil {
		return ErrUnsafe
	}
	f, err := root.OpenFile(".install-next", os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return ErrRecovery
	}
	if f.Chmod(mode) != nil || !regular(f, 0, mode, 0) {
		f.Close()
		return ErrRecovery
	}
	n, writeErr := f.Write(next)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(next) || syncErr != nil || closeErr != nil {
		return ErrRecovery
	}
	after, err := root.Lstat(name)
	if err != nil || !os.SameFile(before, after) {
		return ErrRecovery
	}
	if root.Rename(".install-next", name) != nil || parent.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
