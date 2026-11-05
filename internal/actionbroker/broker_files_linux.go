//go:build linux

package actionbroker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

type brokerRecordKind uint8

const (
	engineRecord brokerRecordKind = iota + 1
	policyRecord
)

type pinnedBrokerRecord struct {
	fds       []int
	witnesses []unix.Stat_t
	names     []string
	limit     int64
	afterRead func() // Private read-window fault observer; production always nil.
}
type brokerFiles struct {
	identity                     actionprofile.Identity
	engine                       engineApproval
	policy                       policyDocument
	policyDigest                 [32]byte
	engineWitness, policyWitness []unix.Stat_t
}

func brokerNoACL(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Fgetxattr(fd, name, nil)
		if errors.Is(err, unix.ENODATA) || errors.Is(err, unix.ENOTSUP) {
			continue
		}
		if err != nil || n != 0 {
			return false
		}
	}
	return true
}
func brokerSame(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
func (p *pinnedBrokerRecord) close() error {
	if p == nil {
		return nil
	}
	fds := p.fds
	p.fds = nil
	failed := false
	for i := len(fds) - 1; i >= 0; i-- {
		if unix.Close(fds[i]) != nil {
			failed = true
		}
	}
	if failed {
		return ErrPolicy
	}
	return nil
}
func (p *pinnedBrokerRecord) recheck(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || p == nil || len(p.fds) != 4 || len(p.witnesses) != 4 {
		return ErrPolicy
	}
	for index, fd := range p.fds {
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || !brokerSame(stat, p.witnesses[index]) || !brokerNoACL(fd) {
			return ErrPolicy
		}
		if index > 0 {
			if unix.Fstatat(p.fds[index-1], p.names[index-1], &stat, unix.AT_SYMLINK_NOFOLLOW) != nil || !brokerSame(stat, p.witnesses[index]) {
				return ErrPolicy
			}
		}
	}
	// An interrupted stopped-policy publication is never silently adopted.
	var stage unix.Stat_t
	if err := unix.Fstatat(p.fds[2], "policy.json.stage", &stage, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return ErrPolicy
	}
	if ctx.Err() != nil {
		return ErrPolicy
	}
	return nil
}

// Only two fixed three-component plans exist. This private root-FD seam permits
// unprivileged synthetic ownership fixtures; production always opens '/' and
// supplies root UID/GID0. No path, owner or validation override is exported.
func pinBrokerRecordAt(ctx context.Context, root int, kind brokerRecordKind, rootUID, rootGID uint32, identity *actionprofile.Identity) (_ *pinnedBrokerRecord, result error) {
	p := &pinnedBrokerRecord{fds: []int{root}}
	defer func() {
		if result != nil {
			p.close()
		}
	}()
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrPolicy
	}
	leaf := "engine.json"
	p.limit = engineDocumentLimit
	if kind == policyRecord {
		leaf = "policy.json"
		p.limit = policyBytes
		if identity == nil {
			return nil, ErrPolicy
		}
	} else if kind != engineRecord {
		return nil, ErrPolicy
	}
	p.names = []string{"etc", "home-lab-observer-actions", leaf}
	var stat unix.Stat_t
	if unix.Fstat(root, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&007022 != 0 || stat.Uid != rootUID || stat.Gid != rootGID || !brokerNoACL(root) {
		return nil, ErrPolicy
	}
	p.witnesses = append(p.witnesses, stat)
	for index, name := range p.names {
		if ctx.Err() != nil {
			return nil, ErrPolicy
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if index < 2 {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(p.fds[len(p.fds)-1], name, flags, 0)
		if err != nil {
			return nil, ErrPolicy
		}
		p.fds = append(p.fds, fd)
		if unix.Fstat(fd, &stat) != nil || stat.Uid != rootUID || stat.Mode&07000 != 0 || !brokerNoACL(fd) {
			return nil, ErrPolicy
		}
		switch index {
		case 0:
			if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0022 != 0 || stat.Gid != rootGID {
				return nil, ErrPolicy
			}
		case 1:
			if stat.Mode != unix.S_IFDIR|0750 || identity != nil && stat.Gid != identity.IPCGID {
				return nil, ErrPolicy
			}
		case 2:
			if stat.Mode != unix.S_IFREG|0640 || stat.Nlink != 1 || stat.Size < 1 || stat.Size > p.limit {
				return nil, ErrPolicy
			}
			if identity != nil && stat.Gid != identity.BrokerGID {
				return nil, ErrPolicy
			}
		}
		p.witnesses = append(p.witnesses, stat)
	}
	if p.recheck(ctx) != nil {
		return nil, ErrPolicy
	}
	return p, nil
}
func (p *pinnedBrokerRecord) read(ctx context.Context) ([]byte, error) {
	if p.recheck(ctx) != nil {
		return nil, ErrPolicy
	}
	raw, err := p.capture()
	if err != nil {
		return nil, ErrPolicy
	}
	if p.afterRead != nil {
		p.afterRead()
	}
	if p.recheck(ctx) != nil {
		clear(raw)
		return nil, ErrPolicy
	}
	// Some supported filesystems expose coarse change timestamps: matching stat
	// fields alone cannot detect a same-size rewrite during capture. Require two
	// bounded preads of the same pinned file to agree, with all witnesses checked
	// around them. This is not an atomic publication/snapshot against root writes.
	again, err := p.capture()
	defer clear(again)
	if err != nil || !bytes.Equal(raw, again) || p.recheck(ctx) != nil {
		clear(raw)
		return nil, ErrPolicy
	}
	return raw, nil
}

func (p *pinnedBrokerRecord) capture() ([]byte, error) {
	dup, err := unix.FcntlInt(uintptr(p.fds[3]), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, ErrPolicy
	}
	f := os.NewFile(uintptr(dup), "action-broker-root-record")
	// A duplicate shares an open-file-description offset. Use bounded pread so
	// a repeated validation cannot mistake a stale EOF for the approved document.
	raw, readErr := io.ReadAll(io.NewSectionReader(f, 0, p.limit+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || int64(len(raw)) != p.witnesses[3].Size || len(raw) == 0 || int64(len(raw)) > p.limit {
		clear(raw)
		return nil, ErrPolicy
	}
	return raw, nil
}

func openBrokerRecord(ctx context.Context, kind brokerRecordKind, identity *actionprofile.Identity) (*pinnedBrokerRecord, error) {
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrPolicy
	}
	var stat unix.Stat_t
	if unix.Fstat(root, &stat) != nil || stat.Mode != unix.S_IFDIR|0755 || stat.Uid != 0 || stat.Gid != 0 {
		unix.Close(root)
		return nil, ErrPolicy
	}
	return pinBrokerRecordAt(ctx, root, kind, 0, 0, identity)
}

// loadBrokerFiles reads only already-provisioned root sources. Initial Engine
// metadata is NOT approval inferred from a live node. Both fixed documents and
// their entire ancestor witnesses stay pinned through exact role resolution.
func loadBrokerFiles(ctx context.Context) (files *brokerFiles, result error) {
	if ctx == nil || ctx.Err() != nil || actionprofile.VerifyHostNamespace() != nil {
		return nil, ErrPolicy
	}
	enginePin, err := openBrokerRecord(ctx, engineRecord, nil)
	if err != nil {
		return nil, ErrPolicy
	}
	defer func() {
		if enginePin.close() != nil {
			files = nil
			result = ErrPolicy
		}
	}()
	raw, err := enginePin.read(ctx)
	if err != nil {
		return nil, ErrPolicy
	}
	defer clear(raw)
	engine, err := decodeEngineApproval(raw)
	if err != nil {
		return nil, ErrPolicy
	}
	identity, err := actionprofile.VerifyBroker(engine.witness.GroupID)
	if err != nil || enginePin.witnesses[2].Gid != identity.IPCGID || enginePin.witnesses[3].Gid != identity.BrokerGID {
		return nil, ErrPolicy
	}
	policyPin, err := openBrokerRecord(ctx, policyRecord, &identity)
	if err != nil {
		return nil, ErrPolicy
	}
	defer func() {
		if policyPin.close() != nil {
			files = nil
			result = ErrPolicy
		}
	}()
	policyRaw, err := policyPin.read(ctx)
	if err != nil {
		return nil, ErrPolicy
	}
	defer clear(policyRaw)
	policy, err := decodePolicy(policyRaw)
	if err != nil || validateEnginePolicy(policy, engine.generation) != nil || enginePin.recheck(ctx) != nil || policyPin.recheck(ctx) != nil {
		return nil, ErrPolicy
	}
	again, err := actionprofile.VerifyBroker(engine.witness.GroupID)
	if err != nil || again != identity || ctx.Err() != nil {
		return nil, ErrPolicy
	}
	return &brokerFiles{identity: identity, engine: engine, policy: policy, policyDigest: sha256.Sum256(policyRaw), engineWitness: append([]unix.Stat_t(nil), enginePin.witnesses...), policyWitness: append([]unix.Stat_t(nil), policyPin.witnesses...)}, nil
}

func brokerCurrentWitnessSame(index int, before, after unix.Stat_t) bool {
	if index == 3 {
		return brokerSame(before, after) // Protected file remains fully immutable.
	}
	// Each separate load still validates full pinned witnesses/ACLs during its
	// read. Across loads, unrelated directory entries are not security identity.
	return index >= 0 && index < 3 && before.Mode&unix.S_IFMT == unix.S_IFDIR &&
		before.Dev == after.Dev && before.Ino == after.Ino && before.Mode == after.Mode && before.Uid == after.Uid && before.Gid == after.Gid
}

// Current is a tripwire, not hot reload. This fixed first profile cannot adopt
// changed approvals; retained mappings never become current Engine authority.
func (f *brokerFiles) Current(ctx context.Context) (policyDocument, error) {
	if f == nil {
		return policyDocument{}, ErrPolicy
	}
	current, err := loadBrokerFiles(ctx)
	if err != nil || current.identity != f.identity || current.engine.generation != f.engine.generation || current.engine.witness != f.engine.witness || current.engine.digest != f.engine.digest || current.policyDigest != f.policyDigest ||
		len(current.engineWitness) != len(f.engineWitness) || len(current.policyWitness) != len(f.policyWitness) {
		return policyDocument{}, ErrPolicy
	}
	for i, old := range f.engineWitness {
		if !brokerCurrentWitnessSame(i, old, current.engineWitness[i]) {
			return policyDocument{}, ErrPolicy
		}
	}
	for i, old := range f.policyWitness {
		if !brokerCurrentWitnessSame(i, old, current.policyWitness[i]) {
			return policyDocument{}, ErrPolicy
		}
	}
	return current.policy, nil
}
