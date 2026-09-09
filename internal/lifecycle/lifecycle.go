// Package lifecycle implements a fixed owner-only, nonce-bound graceful-stop
// channel for a managed local observer. It does not use HTTP, process IDs, or
// accept commands.
package lifecycle

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

const (
	controlDirectory = "lifecycle"
	instanceName     = "instance.json"
	requestName      = "stop.request"
	lockName         = "lifecycle.lock"
	instanceTempName = "instance.tmp"
	requestTempName  = "stop.request.tmp"
	maxControlBytes  = int64(1024)
	nonceBytes       = 32
)

var (
	ErrAlreadyRunning = errors.New("managed observer is already running")
	ErrNotRunning     = errors.New("managed observer is not running")
	ErrUnsafePath     = errors.New("unsafe lifecycle path")
	ErrMalformedState = errors.New("malformed lifecycle state")
	ErrWrongInstance  = errors.New("lifecycle request targets another instance")
	requestMu         sync.Mutex
)

type Config struct {
	StateDir     string
	Now          func() time.Time
	Random       io.Reader
	PollInterval time.Duration
}

type Endpoint struct {
	directory string
	nonce     string
	interval  time.Duration
	requested chan struct{}
	done      chan struct{}
	stopped   chan struct{}
	once      sync.Once
	closeOnce sync.Once
	lock      *fileLock
	closeErr  error
}

type instanceFile struct {
	SchemaVersion string `json:"schema_version"`
	Nonce         string `json:"nonce"`
	StartedAt     string `json:"started_at"`
}

type requestFile struct {
	SchemaVersion string `json:"schema_version"`
	Nonce         string `json:"nonce"`
}

func New(config Config) (*Endpoint, error) {
	directory, err := ownerfs.EnsurePrivateSubdir(config.StateDir, controlDirectory)
	if err != nil {
		return nil, classifyPath(err)
	}
	if err := validateDirectoryEntries(directory); err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(filepath.Join(directory, lockName))
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()

	if err := cleanupTemp(directory, instanceTempName); err != nil {
		return nil, err
	}
	if err := cleanupTemp(directory, requestTempName); err != nil {
		return nil, err
	}
	if _, err := readInstance(filepath.Join(directory, instanceName)); err == nil {
		if err := os.Remove(filepath.Join(directory, instanceName)); err != nil {
			return nil, classifyPath(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if _, err := readRequest(filepath.Join(directory, requestName)); err == nil {
		if err := os.Remove(filepath.Join(directory, requestName)); err != nil {
			return nil, classifyPath(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	random := config.Random
	if random == nil {
		random = rand.Reader
	}
	rawNonce := make([]byte, nonceBytes)
	if _, err := io.ReadFull(random, rawNonce); err != nil {
		return nil, fmt.Errorf("generate instance nonce: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(rawNonce)
	now := config.Now
	if now == nil {
		now = time.Now
	}
	contents, err := json.Marshal(instanceFile{
		SchemaVersion: "observer-lifecycle-instance/v1",
		Nonce:         nonce,
		StartedAt:     now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(directory, instanceTempName, instanceName, append(contents, '\n')); err != nil {
		return nil, err
	}

	interval := config.PollInterval
	if interval == 0 {
		interval = 100 * time.Millisecond
	}
	if interval < 10*time.Millisecond || interval > time.Second {
		_ = removeOwnInstance(directory, nonce)
		return nil, errors.New("poll interval must be between 10ms and 1s")
	}
	endpoint := &Endpoint{
		directory: directory, nonce: nonce, interval: interval,
		requested: make(chan struct{}), done: make(chan struct{}), stopped: make(chan struct{}), lock: lock,
	}
	keepLock = true
	go endpoint.watch()
	return endpoint, nil
}

func (e *Endpoint) StopRequested() <-chan struct{} { return e.requested }

func (e *Endpoint) watch() {
	defer close(e.stopped)
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			request, err := readRequest(filepath.Join(e.directory, requestName))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil || request.Nonce != e.nonce {
				continue
			}
			if err := os.Remove(filepath.Join(e.directory, requestName)); err != nil {
				continue
			}
			e.once.Do(func() { close(e.requested) })
			return
		}
	}
}

func (e *Endpoint) Close() error {
	return e.finish(true)
}

// Abandon releases in-process resources after an unclean runtime shutdown but
// deliberately preserves the instance publication. A stop requester therefore
// times out instead of mistaking an incomplete history/listener drain for
// success. The next exclusively locked New validates and replaces the stale
// publication.
func (e *Endpoint) Abandon() error {
	return e.finish(false)
}

func (e *Endpoint) finish(clean bool) error {
	e.closeOnce.Do(func() {
		close(e.done)
		<-e.stopped
		if clean {
			err1 := removeOwnRequest(e.directory, e.nonce)
			err2 := removeOwnInstance(e.directory, e.nonce)
			e.closeErr = errors.Join(err1, err2)
		}
		e.closeErr = errors.Join(e.closeErr, e.lock.Close())
	})
	return e.closeErr
}

// RequestStop publishes a request for the currently published instance and
// waits until that exact instance removes its publication during clean close.
func RequestStop(ctx context.Context, stateDir string) error {
	requestMu.Lock()
	defer requestMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := ownerfs.OpenPrivateSubdir(stateDir, controlDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotRunning
	}
	if err != nil {
		return classifyPath(err)
	}
	if err := validateDirectoryEntries(directory); err != nil {
		return err
	}
	instance, err := readInstance(filepath.Join(directory, instanceName))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotRunning
	}
	if err != nil {
		return err
	}
	requestPath := filepath.Join(directory, requestName)
	request, err := readRequest(requestPath)
	if err == nil {
		if request.Nonce != instance.Nonce {
			return ErrWrongInstance
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := cleanupTemp(directory, requestTempName); err != nil {
			return err
		}
		contents, marshalErr := json.Marshal(requestFile{SchemaVersion: "observer-lifecycle-stop/v1", Nonce: instance.Nonce})
		if marshalErr != nil {
			return marshalErr
		}
		if err := writeAtomic(directory, requestTempName, requestName, append(contents, '\n')); err != nil {
			return err
		}
	} else {
		return err
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, err := readInstance(filepath.Join(directory, instanceName))
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if current.Nonce != instance.Nonce {
				return ErrWrongInstance
			}
		}
	}
}

func validateDirectoryEntries(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return classifyPath(err)
	}
	allowed := map[string]bool{instanceName: true, requestName: true, lockName: true, instanceTempName: true, requestTempName: true}
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if _, err := ownerfs.ValidateRegular(filepath.Join(directory, entry.Name()), maxControlBytes); err != nil {
			return classifyPath(err)
		}
	}
	return nil
}

func cleanupTemp(directory, name string) error {
	path := filepath.Join(directory, name)
	if _, err := ownerfs.ValidateRegular(path, maxControlBytes); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return classifyPath(err)
	}
	if err := os.Remove(path); err != nil {
		return classifyPath(err)
	}
	return nil
}

func readInstance(path string) (instanceFile, error) {
	var value instanceFile
	if err := readExact(path, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != "observer-lifecycle-instance/v1" || !validNonce(value.Nonce) {
		return value, ErrMalformedState
	}
	if _, err := time.Parse(time.RFC3339Nano, value.StartedAt); err != nil {
		return value, ErrMalformedState
	}
	return value, nil
}

func readRequest(path string) (requestFile, error) {
	var value requestFile
	if err := readExact(path, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != "observer-lifecycle-stop/v1" || !validNonce(value.Nonce) {
		return value, ErrMalformedState
	}
	return value, nil
}

func readExact(path string, destination any) error {
	info, err := ownerfs.ValidateRegular(path, maxControlBytes)
	if err != nil {
		return classifyPath(err)
	}
	file, err := os.Open(path)
	if err != nil {
		return classifyPath(err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ErrUnsafePath
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxControlBytes+1))
	if err != nil || int64(len(contents)) > maxControlBytes {
		return ErrMalformedState
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrMalformedState
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrMalformedState
	}
	return nil
}

func validNonce(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == nonceBytes && base64.RawURLEncoding.EncodeToString(raw) == value
}

func writeAtomic(directory, temporaryName, finalName string, contents []byte) error {
	if int64(len(contents)) > maxControlBytes {
		return ErrMalformedState
	}
	temporary := filepath.Join(directory, temporaryName)
	final := filepath.Join(directory, finalName)
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return classifyPath(err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = file.Close()
			_ = os.Remove(temporary)
		}
	}()
	if err := ownerfs.RestrictFile(temporary); err != nil {
		return classifyPath(err)
	}
	written, err := file.Write(contents)
	if err == nil && written != len(contents) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(final); err == nil {
		return ErrWrongInstance
	} else if !errors.Is(err, os.ErrNotExist) {
		return classifyPath(err)
	}
	if err := os.Rename(temporary, final); err != nil {
		return classifyPath(err)
	}
	complete = true
	return nil
}

func removeOwnInstance(directory, nonce string) error {
	path := filepath.Join(directory, instanceName)
	value, err := readInstance(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if value.Nonce != nonce {
		return ErrWrongInstance
	}
	return os.Remove(path)
}

func removeOwnRequest(directory, nonce string) error {
	path := filepath.Join(directory, requestName)
	value, err := readRequest(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if value.Nonce != nonce {
		return ErrWrongInstance
	}
	return os.Remove(path)
}

func classifyPath(err error) error {
	if errors.Is(err, ownerfs.ErrUnsafePath) {
		return ErrUnsafePath
	}
	return err
}
