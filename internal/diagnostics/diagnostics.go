// Package diagnostics writes a deliberately small, code-owned diagnostic
// vocabulary to bounded owner-only JSONL files. It cannot accept messages,
// errors, paths, request data, or observed identities.
package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

const (
	MaxFiles       = 5
	MaxFileBytes   = int64(2 * 1024 * 1024)
	MaxTotalBytes  = int64(MaxFiles) * MaxFileBytes
	MaxRecordBytes = int64(8 * 1024)
	MaxAgeSeconds  = int64(7 * 24 * 60 * 60)
)

type EventKind string

const (
	EventRuntimeStarted     EventKind = "RUNTIME_STARTED"
	EventRuntimeReady       EventKind = "RUNTIME_READY"
	EventStopRequested      EventKind = "STOP_REQUESTED"
	EventRuntimeStopped     EventKind = "RUNTIME_STOPPED"
	EventCollectionCycle    EventKind = "COLLECTION_CYCLE"
	EventHistoryMaintenance EventKind = "HISTORY_MAINTENANCE"
	EventHTTPServer         EventKind = "HTTP_SERVER"
)

type ResultCode string

const (
	CodeOK               ResultCode = "OK"
	CodePartial          ResultCode = "PARTIAL"
	CodeFailed           ResultCode = "FAILED"
	CodeTimeout          ResultCode = "TIMEOUT"
	CodeUnavailable      ResultCode = "UNAVAILABLE"
	CodePermissionDenied ResultCode = "PERMISSION_DENIED"
	CodeDisabled         ResultCode = "DISABLED"
	CodeCancelled        ResultCode = "CANCELLED"
)

type Config struct {
	StateDir string
	Enabled  bool
	Now      func() time.Time
}

type Event struct {
	Kind     EventKind
	Code     ResultCode
	Version  string
	Count    uint64
	Duration time.Duration
}

type Health struct {
	Enabled        bool
	Available      bool
	State          string
	ReasonCode     string
	MaxFiles       int
	MaxFileBytes   int64
	MaxTotalBytes  int64
	MaxRecordBytes int64
	MaxAgeSeconds  int64
	TotalBytes     int64
	FileCount      int
	DroppedRecords uint64
	WriteFailures  uint64
}

type Writer struct {
	mu        sync.Mutex
	directory string
	now       func() time.Time
	openFile  func(string, int, os.FileMode) (*os.File, error)
	done      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
	closed    bool
	health    Health
}

type diskRecord struct {
	ObservedAt string     `json:"observed_at"`
	Event      EventKind  `json:"event"`
	Code       ResultCode `json:"code"`
	Version    string     `json:"version"`
	Count      uint64     `json:"count"`
	DurationMS int64      `json:"duration_ms"`
}

var safeVersion = regexp.MustCompile(`^[0-9A-Za-z.+-]{1,64}$`)

var eventKinds = map[EventKind]struct{}{
	EventRuntimeStarted: {}, EventRuntimeReady: {}, EventStopRequested: {}, EventRuntimeStopped: {},
	EventCollectionCycle: {}, EventHistoryMaintenance: {}, EventHTTPServer: {},
}

var resultCodes = map[ResultCode]struct{}{
	CodeOK: {}, CodePartial: {}, CodeFailed: {}, CodeTimeout: {}, CodeUnavailable: {},
	CodePermissionDenied: {}, CodeDisabled: {}, CodeCancelled: {},
}

func New(config Config) *Writer {
	health := Health{
		Enabled: config.Enabled, MaxFiles: MaxFiles, MaxFileBytes: MaxFileBytes,
		MaxTotalBytes: MaxTotalBytes, MaxRecordBytes: MaxRecordBytes, MaxAgeSeconds: MaxAgeSeconds,
	}
	if !config.Enabled {
		health.State, health.ReasonCode = "DISABLED", "DIAGNOSTICS_DISABLED"
		return &Writer{now: normalizeClock(config.Now), openFile: os.OpenFile, health: health}
	}
	directory, err := ownerfs.EnsurePrivateSubdir(config.StateDir, "diagnostics")
	writer := &Writer{directory: directory, now: normalizeClock(config.Now), openFile: os.OpenFile, health: health}
	if err != nil {
		writer.unavailable(errors.Is(err, ownerfs.ErrUnsafePath))
		return writer
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := writer.maintainLocked(); err != nil {
		writer.unavailable(errors.Is(err, ownerfs.ErrUnsafePath))
		return writer
	}
	writer.health.Available = true
	writer.health.State = "AVAILABLE"
	writer.done, writer.stopped = make(chan struct{}), make(chan struct{})
	go writer.housekeepingLoop()
	return writer
}

func normalizeClock(now func() time.Time) func() time.Time {
	if now == nil {
		return time.Now
	}
	return now
}

func (w *Writer) Record(event Event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.health.Enabled || w.closed {
		return
	}
	if !validEvent(event) {
		w.health.DroppedRecords++
		return
	}
	record := diskRecord{
		ObservedAt: w.now().UTC().Format(time.RFC3339Nano), Event: event.Kind, Code: event.Code,
		Version: event.Version, Count: event.Count, DurationMS: event.Duration.Milliseconds(),
	}
	line, err := json.Marshal(record)
	if err != nil || int64(len(line)+1) > MaxRecordBytes {
		w.health.DroppedRecords++
		return
	}
	line = append(line, '\n')
	if err := w.maintainLocked(); err != nil {
		w.recordWriteFailure(errors.Is(err, ownerfs.ErrUnsafePath))
		return
	}
	if err := w.rotateForLocked(int64(len(line))); err != nil {
		w.recordWriteFailure(errors.Is(err, ownerfs.ErrUnsafePath))
		return
	}
	path := filepath.Join(w.directory, "observer.jsonl")
	file, err := w.openFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		w.recordWriteFailure(false)
		return
	}
	err = ownerfs.RestrictFile(path)
	if err == nil {
		var written int
		written, err = file.Write(line)
		if err == nil && written != len(line) {
			err = io.ErrShortWrite
		}
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		w.recordWriteFailure(false)
		return
	}
	w.health.Available, w.health.State, w.health.ReasonCode = true, "AVAILABLE", ""
	if err := w.refreshUsageLocked(); err != nil {
		w.unavailable(errors.Is(err, ownerfs.ErrUnsafePath))
	}
}

func validEvent(event Event) bool {
	_, validKind := eventKinds[event.Kind]
	_, validCode := resultCodes[event.Code]
	return validKind && validCode && safeVersion.MatchString(event.Version) && event.Duration >= 0 && event.Duration <= time.Hour
}

func (w *Writer) Health() Health {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.health
}

func (w *Writer) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		done, stopped := w.done, w.stopped
		w.mu.Unlock()
		if done != nil {
			close(done)
			<-stopped
		}
	})
	return nil
}

func (w *Writer) housekeepingLoop() {
	defer close(w.stopped)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.housekeep()
		}
	}
}

func (w *Writer) housekeep() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || !w.health.Enabled || w.directory == "" {
		return
	}
	if err := w.maintainLocked(); err != nil {
		w.unavailable(errors.Is(err, ownerfs.ErrUnsafePath))
		return
	}
	w.health.Available, w.health.State, w.health.ReasonCode = true, "AVAILABLE", ""
}

func (w *Writer) unavailable(unsafe bool) {
	w.health.Available = false
	w.health.State = "UNAVAILABLE"
	w.health.ReasonCode = "DIAGNOSTICS_UNAVAILABLE"
	if unsafe {
		w.health.ReasonCode = "UNSAFE_DIAGNOSTICS_PATH"
	}
	w.health.WriteFailures++
}

func (w *Writer) recordWriteFailure(unsafe bool) {
	w.health.DroppedRecords++
	w.unavailable(unsafe)
}

func (w *Writer) maintainLocked() error {
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return err
	}
	cutoff := w.now().UTC().Add(-time.Duration(MaxAgeSeconds) * time.Second)
	for _, entry := range entries {
		index, ok := diagnosticIndex(entry.Name())
		if !ok || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return ownerfs.ErrUnsafePath
		}
		path := filepath.Join(w.directory, diagnosticName(index))
		info, err := ownerfs.ValidateRegular(path, MaxFileBytes)
		if err != nil {
			return err
		}
		oldest, err := oldestRecordTime(path, info)
		if err != nil {
			return err
		}
		if oldest.Before(cutoff) {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return w.refreshUsageLocked()
}

func oldestRecordTime(path string, info os.FileInfo) (time.Time, error) {
	if info.Size() == 0 {
		return info.ModTime().UTC(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()
	line, err := io.ReadAll(io.LimitReader(file, MaxRecordBytes+1))
	if err != nil {
		return time.Time{}, err
	}
	newline := -1
	for index, value := range line {
		if value == '\n' {
			newline = index
			break
		}
	}
	if newline < 0 || int64(newline+1) > MaxRecordBytes {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	var record diskRecord
	decoder := json.NewDecoder(bytes.NewReader(line[:newline]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	if _, ok := eventKinds[record.Event]; !ok {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	if _, ok := resultCodes[record.Code]; !ok || !safeVersion.MatchString(record.Version) || record.DurationMS < 0 || record.DurationMS > time.Hour.Milliseconds() {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	at, err := time.Parse(time.RFC3339Nano, record.ObservedAt)
	if err != nil {
		return time.Time{}, ownerfs.ErrUnsafePath
	}
	return at.UTC(), nil
}

func (w *Writer) rotateForLocked(incoming int64) error {
	active := filepath.Join(w.directory, diagnosticName(0))
	info, err := ownerfs.ValidateRegular(active, MaxFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size()+incoming <= MaxFileBytes {
		return nil
	}
	oldest := filepath.Join(w.directory, diagnosticName(MaxFiles-1))
	if _, err := ownerfs.ValidateRegular(oldest, MaxFileBytes); err == nil {
		if err := os.Remove(oldest); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for index := MaxFiles - 2; index >= 0; index-- {
		source := filepath.Join(w.directory, diagnosticName(index))
		if _, err := ownerfs.ValidateRegular(source, MaxFileBytes); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Rename(source, filepath.Join(w.directory, diagnosticName(index+1))); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) refreshUsageLocked() error {
	entries, err := os.ReadDir(w.directory)
	if err != nil {
		return err
	}
	var total int64
	count := 0
	for _, entry := range entries {
		index, ok := diagnosticIndex(entry.Name())
		if !ok {
			return ownerfs.ErrUnsafePath
		}
		info, err := ownerfs.ValidateRegular(filepath.Join(w.directory, diagnosticName(index)), MaxFileBytes)
		if err != nil {
			return err
		}
		total += info.Size()
		count++
	}
	if total > MaxTotalBytes || count > MaxFiles {
		return ownerfs.ErrUnsafePath
	}
	w.health.TotalBytes, w.health.FileCount = total, count
	return nil
}

func diagnosticName(index int) string {
	if index == 0 {
		return "observer.jsonl"
	}
	return "observer.jsonl." + string(rune('0'+index))
}

func diagnosticIndex(name string) (int, bool) {
	if name == "observer.jsonl" {
		return 0, true
	}
	if len(name) == len("observer.jsonl.1") && name[:len("observer.jsonl.")] == "observer.jsonl." {
		index := int(name[len(name)-1] - '0')
		return index, index >= 1 && index < MaxFiles
	}
	return 0, false
}
