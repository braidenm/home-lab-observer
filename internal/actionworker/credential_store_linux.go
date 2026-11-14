//go:build linux

package actionworker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

type credentialFile struct {
	raw     []byte
	witness unix.Stat_t
	present bool
}

func (credentialFile) String() string   { return "ActionCredentialFile([redacted])" }
func (credentialFile) GoString() string { return "ActionCredentialFile([redacted])" }

type credentialStore struct {
	binding                 actionremote.Binding
	fds                     []int
	parts                   []protectedPart
	dirs                    []unix.Stat_t
	lock                    int
	lockStat                unix.Stat_t
	uid, gid                uint32
	handoffFile, activeFile credentialFile
	loaded, poisoned        bool
	// Private filesystem fault boundary; never supplied by production.
	fault func(string) error
}

func (*credentialStore) String() string   { return "ActionCredentialStore([redacted])" }
func (*credentialStore) GoString() string { return "ActionCredentialStore([redacted])" }

func openCredentialStore(ctx context.Context, i actionprofile.Identity, b actionremote.Binding) (*credentialStore, error) {
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	var stat unix.Stat_t
	if unix.Fstat(root, &stat) != nil || stat.Uid != 0 || stat.Gid != 0 {
		_ = unix.Close(root)
		return nil, ErrUnsafe
	}
	parts := activeParts(i)
	return openCredentialStoreAt(ctx, root, parts[:len(parts)-1], b)
}

// Takes ownership of root; only private fixtures may substitute its descriptor.
func openCredentialStoreAt(ctx context.Context, root int, parts []protectedPart, b actionremote.Binding) (_ *credentialStore, result error) {
	s := &credentialStore{binding: b, fds: []int{root}, parts: parts, lock: -1}
	defer func() {
		if result != nil {
			s.close()
		}
	}()
	if ctx == nil || ctx.Err() != nil || len(parts) < 1 || len(parts) > 4 {
		return nil, ErrUnsafe
	}
	var stat unix.Stat_t
	if unix.Fstat(root, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&07022 != 0 || !noACL(root) {
		return nil, ErrUnsafe
	}
	// Production starts at root:root; fixtures start at an owned private root.
	if stat.Uid != 0 && stat.Uid != uint32(os.Geteuid()) {
		return nil, ErrUnsafe
	}
	s.dirs = append(s.dirs, stat)
	for _, part := range parts {
		if part.name == "" || part.name == "." || part.name == ".." {
			return nil, ErrUnsafe
		}
		for _, c := range part.name {
			if c == '/' || c == 0 {
				return nil, ErrUnsafe
			}
		}
		fd, err := unix.Openat(s.fds[len(s.fds)-1], part.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, ErrUnsafe
		}
		s.fds = append(s.fds, fd)
		if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != part.uid || stat.Gid != part.gid || stat.Mode&07000 != 0 || !noACL(fd) {
			return nil, ErrUnsafe
		}
		if part.mode == 0 && stat.Mode&0022 != 0 || part.mode != 0 && stat.Mode&07777 != part.mode {
			return nil, ErrUnsafe
		}
		s.dirs = append(s.dirs, stat)
	}
	s.uid, s.gid = parts[len(parts)-1].uid, parts[len(parts)-1].gid
	var err error
	s.lock, err = unix.Openat(s.dir(), "credential.lock", unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil || unix.Fstat(s.lock, &s.lockStat) != nil || !s.validFile(s.lockStat, true) || !noACL(s.lock) || unix.Flock(s.lock, unix.LOCK_EX|unix.LOCK_NB) != nil || s.current(ctx) != nil {
		return nil, ErrUnsafe
	}
	return s, nil
}
func (s *credentialStore) dir() int { return s.fds[len(s.fds)-1] }
func (s *credentialStore) close() {
	if s == nil {
		return
	}
	clear(s.handoffFile.raw)
	clear(s.activeFile.raw)
	if s.lock >= 0 {
		unix.Close(s.lock)
		s.lock = -1
	}
	for _, fd := range s.fds {
		unix.Close(fd)
	}
	s.fds = nil
}
func stableCredentialDirectory(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid
}
func (s *credentialStore) validFile(st unix.Stat_t, lock bool) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Mode&07777 == 0600 && st.Uid == s.uid && st.Gid == s.gid && st.Nlink == 1 && ((lock && st.Size == 0) || (!lock && st.Size > 0 && st.Size <= documentLimit))
}
func (s *credentialStore) current(ctx context.Context) error {
	if s == nil || s.poisoned || ctx == nil || ctx.Err() != nil || len(s.fds) == 0 || s.lock < 0 {
		return ErrUnsafe
	}
	for j, fd := range s.fds {
		var now unix.Stat_t
		if unix.Fstat(fd, &now) != nil || !stableCredentialDirectory(now, s.dirs[j]) || !noACL(fd) {
			return ErrUnsafe
		}
		if j > 0 && (unix.Fstatat(s.fds[j-1], s.parts[j-1].name, &now, unix.AT_SYMLINK_NOFOLLOW) != nil || !stableCredentialDirectory(now, s.dirs[j])) {
			return ErrUnsafe
		}
	}
	var st unix.Stat_t
	if unix.Fstat(s.lock, &st) != nil || !sameWitness(st, s.lockStat) || !noACL(s.lock) || unix.Fstatat(s.dir(), "credential.lock", &st, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameWitness(st, s.lockStat) || ctx.Err() != nil {
		return ErrUnsafe
	}
	return nil
}
func (s *credentialStore) read(ctx context.Context, name string, optional bool) (credentialFile, error) {
	if s.current(ctx) != nil {
		return credentialFile{}, ErrUnsafe
	}
	fd, err := unix.Openat(s.dir(), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT && optional {
		return credentialFile{}, nil
	}
	if err != nil {
		return credentialFile{}, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "action-credential-state")
	defer f.Close()
	var st, after unix.Stat_t
	if unix.Fstat(fd, &st) != nil || !s.validFile(st, false) || !noACL(fd) {
		return credentialFile{}, ErrUnsafe
	}
	raw, err := io.ReadAll(io.LimitReader(f, documentLimit+1))
	verify, verifyErr := io.ReadAll(io.NewSectionReader(f, 0, documentLimit+1))
	defer clear(verify)
	if err != nil || verifyErr != nil || !bytes.Equal(raw, verify) || int64(len(raw)) != st.Size || unix.Fstat(fd, &after) != nil || !sameWitness(st, after) || !noACL(fd) || unix.Fstatat(s.dir(), name, &after, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameWitness(st, after) || s.current(ctx) != nil {
		clear(raw)
		return credentialFile{}, ErrUnsafe
	}
	return credentialFile{raw, st, true}, nil
}
func sameCredentialFile(a, b credentialFile) bool {
	return a.present == b.present && (!a.present || sameWitness(a.witness, b.witness) && bytes.Equal(a.raw, b.raw))
}
func (s *credentialStore) load(ctx context.Context) (handoffDocument, []byte, error) {
	h, err := s.read(ctx, "handoff.json", false)
	if err != nil {
		return handoffDocument{}, nil, ErrUnsafe
	}
	a, err := s.read(ctx, "active.json", true)
	if err != nil {
		clear(h.raw)
		return handoffDocument{}, nil, ErrUnsafe
	}
	state, err := decodeHandoff(h.raw, s.binding)
	if err != nil || validateActiveWitness(state, a.raw, s.binding) != nil || s.loaded && (!sameCredentialFile(h, s.handoffFile) || !sameCredentialFile(a, s.activeFile)) {
		clear(h.raw)
		clear(a.raw)
		return handoffDocument{}, nil, ErrUnsafe
	}
	clear(s.handoffFile.raw)
	clear(s.activeFile.raw)
	s.handoffFile, s.activeFile, s.loaded = h, a, true
	return state, bytes.Clone(a.raw), nil
}
func (s *credentialStore) handoff(op *credentialOperation, h handoffDocument) error {
	raw, err := encodeHandoff(h, s.binding)
	if err != nil {
		return ErrUnsafe
	}
	defer clear(raw)
	return s.publish(op, "handoff.json", raw)
}
func (s *credentialStore) active(op *credentialOperation, c candidateRecord) error {
	raw, err := candidateActiveBytes(c, s.binding)
	if err != nil {
		return ErrUnsafe
	}
	defer clear(raw)
	return s.publish(op, "active.json", raw)
}
func (s *credentialStore) stage(stage string) error {
	if s.fault != nil {
		return s.fault(stage)
	}
	return nil
}
func (s *credentialStore) publish(op *credentialOperation, name string, raw []byte) (result error) {
	if op.check() != nil || !s.loaded || s.current(op.ctx) != nil || name != "handoff.json" && name != "active.json" {
		return ErrUnsafe
	}
	defer func() {
		if result != nil {
			s.poisoned = true
		}
	}()
	// Compare BOTH records, including their inode witnesses, before any mutation.
	for _, pair := range []struct {
		name string
		old  credentialFile
	}{{"handoff.json", s.handoffFile}, {"active.json", s.activeFile}} {
		now, err := s.read(op.ctx, pair.name, !pair.old.present)
		if err != nil || !sameCredentialFile(now, pair.old) {
			clear(now.raw)
			return ErrUnsafe
		}
		clear(now.raw)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ErrUnsafe
	}
	temp := ".credential-" + hex.EncodeToString(nonce[:])
	fd, err := unix.Openat(s.dir(), temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "action-credential-publication")
	defer f.Close()
	var staged unix.Stat_t
	if unix.Fstat(fd, &staged) != nil || staged.Uid != s.uid || staged.Gid != s.gid || staged.Mode&07777 != 0600 || staged.Nlink != 1 || !noACL(fd) {
		return ErrUnsafe
	}
	renamed := false
	defer func() {
		// Only this exclusive inode may be removed, never an unknown file.
		var current unix.Stat_t
		if !renamed && unix.Fstatat(s.dir(), temp, &current, unix.AT_SYMLINK_NOFOLLOW) == nil && current.Dev == staged.Dev && current.Ino == staged.Ino {
			unix.Unlinkat(s.dir(), temp, 0)
		}
	}()
	if s.stage("before-write") != nil || op.check() != nil {
		return ErrUnsafe
	}
	if n, err := f.Write(raw); err != nil || n != len(raw) || f.Sync() != nil || s.stage("after-file-sync") != nil || op.check() != nil || s.current(op.ctx) != nil {
		return ErrUnsafe
	}
	// Recheck the exact destination and both CAS references after slow I/O.
	for _, pair := range []struct {
		name string
		old  credentialFile
	}{{"handoff.json", s.handoffFile}, {"active.json", s.activeFile}} {
		now, err := s.read(op.ctx, pair.name, !pair.old.present)
		if err != nil || !sameCredentialFile(now, pair.old) {
			clear(now.raw)
			return ErrUnsafe
		}
		clear(now.raw)
	}
	var current unix.Stat_t
	if unix.Fstat(fd, &staged) != nil || !s.validFile(staged, false) || !noACL(fd) || unix.Fstatat(s.dir(), temp, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameWitness(current, staged) || s.stage("before-rename") != nil || op.check() != nil {
		return ErrUnsafe
	}
	flags := uint(0)
	if name == "handoff.json" && !s.handoffFile.present || name == "active.json" && !s.activeFile.present {
		flags = unix.RENAME_NOREPLACE
	}
	if unix.Renameat2(s.dir(), temp, s.dir(), name, flags) != nil {
		return ErrUnsafe
	}
	renamed = true
	if s.stage("after-rename") != nil || unix.Fsync(s.dir()) != nil || s.stage("after-directory-sync") != nil || op.check() != nil || s.current(op.ctx) != nil {
		return ErrUnsafe
	}
	now, err := s.read(op.ctx, name, false)
	if err != nil || !bytes.Equal(now.raw, raw) {
		clear(now.raw)
		return ErrUnsafe
	}
	if name == "handoff.json" {
		clear(s.handoffFile.raw)
		s.handoffFile = now
	} else {
		clear(s.activeFile.raw)
		s.activeFile = now
	}
	return nil
}
