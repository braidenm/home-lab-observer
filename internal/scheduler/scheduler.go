package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/projection"
)

type Store interface {
	NextSequence(context.Context) (uint64, error)
	WriteSamples(context.Context, []history.Sample) error
	Maintain(context.Context, time.Time) error
	Health() history.Health
	Close() error
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

type Config struct {
	Interval          time.Duration
	CollectionTimeout time.Duration
	StorageTimeout    time.Duration
	System            projection.SystemInfo
	Clock             Clock
	NewTicker         func(time.Duration) Ticker
}

func DefaultConfig() Config {
	return Config{Interval: 15 * time.Second, CollectionTimeout: 10 * time.Second, StorageTimeout: 5 * time.Second, System: projection.NativeSystemInfo(), Clock: realClock{}, NewTicker: func(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }}
}

type Stats struct{ Collections, CollectionFailures, StoreFailures, TriggersDropped uint64 }

type Scheduler struct {
	collect     func(context.Context) observation.Snapshot
	store       Store
	config      Config
	trigger     chan struct{}
	started     atomic.Bool
	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	mu          sync.RWMutex
	current     projection.CurrentSnapshot
	hasCurrent  bool
	previous    *observation.Snapshot
	stats       Stats
}

func New(collect func(context.Context) observation.Snapshot, store Store, config Config) (*Scheduler, error) {
	if collect == nil || store == nil {
		return nil, errors.New("collector and store are required")
	}
	defaults := DefaultConfig()
	if config.Interval <= 0 {
		config.Interval = defaults.Interval
	}
	if config.CollectionTimeout <= 0 {
		config.CollectionTimeout = defaults.CollectionTimeout
	}
	if config.StorageTimeout <= 0 {
		config.StorageTimeout = defaults.StorageTimeout
	}
	if config.Clock == nil {
		config.Clock = defaults.Clock
	}
	if config.NewTicker == nil {
		config.NewTicker = defaults.NewTicker
	}
	if config.System.OS == "" {
		config.System = defaults.System
	}
	return &Scheduler{collect: collect, store: store, config: config, trigger: make(chan struct{}, 1), done: make(chan struct{})}, nil
}

func (s *Scheduler) Start(parent context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started.Load() {
		return errors.New("scheduler already started")
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started.Store(true)
	go s.loop(ctx)
	return nil
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)
	defer s.store.Close()
	ticker := s.config.NewTicker(s.config.Interval)
	defer ticker.Stop()
	s.collectOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.collectOnce(ctx)
		case <-s.trigger:
			s.collectOnce(ctx)
		}
	}
}

func (s *Scheduler) collectOnce(parent context.Context) {
	collectionCtx, cancelCollection := context.WithTimeout(parent, s.config.CollectionTimeout)
	raw := s.collect(collectionCtx)
	cancelCollection()
	candidate := projection.Current(raw, s.config.System)
	s.mu.Lock()
	s.stats.Collections++
	if candidate.CollectionState == "FAILED" {
		s.stats.CollectionFailures++
		if s.hasCurrent {
			s.mu.Unlock()
			return
		}
	}
	s.mu.Unlock()
	storageCtx, cancelStorage := context.WithTimeout(parent, s.config.StorageTimeout)
	defer cancelStorage()
	sequence, err := s.store.NextSequence(storageCtx)
	if err != nil {
		s.mu.Lock()
		s.stats.StoreFailures++
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	if s.hasCurrent && sequence <= s.current.Sequence {
		s.stats.StoreFailures++
		s.mu.Unlock()
		return
	}
	candidate.Sequence = sequence
	s.current = cloneSnapshot(candidate)
	s.hasCurrent = true
	s.mu.Unlock()
	samples := history.Extract(raw, s.previous)
	if err := s.store.WriteSamples(storageCtx, samples); err != nil {
		s.mu.Lock()
		s.stats.StoreFailures++
		s.mu.Unlock()
	}
	if err := s.store.Maintain(storageCtx, s.config.Clock.Now()); err != nil {
		s.mu.Lock()
		s.stats.StoreFailures++
		s.mu.Unlock()
	}
	s.previous = previousSnapshot(raw)
}

func (s *Scheduler) Trigger() bool {
	select {
	case s.trigger <- struct{}{}:
		return true
	default:
		s.mu.Lock()
		s.stats.TriggersDropped++
		s.mu.Unlock()
		return false
	}
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	if !s.started.Load() {
		s.lifecycleMu.Unlock()
		return nil
	}
	cancel := s.cancel
	done := s.done
	s.lifecycleMu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Scheduler) Current() (projection.CurrentSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.current), s.hasCurrent
}
func (s *Scheduler) Stats() Stats                { s.mu.RLock(); defer s.mu.RUnlock(); return s.stats }
func (s *Scheduler) StoreHealth() history.Health { return s.store.Health() }

func cloneSnapshot(value projection.CurrentSnapshot) projection.CurrentSnapshot {
	clone := value
	clone.Privacy.ExcludedFields = append([]string(nil), value.Privacy.ExcludedFields...)
	clone.Sections.Filesystems.Items = append([]projection.Filesystem(nil), value.Sections.Filesystems.Items...)
	clone.Sections.Processes.Items = append([]projection.Process(nil), value.Sections.Processes.Items...)
	clone.Sections.Services.Items = append([]any(nil), value.Sections.Services.Items...)
	clone.Sections.Containers.Items = append([]any(nil), value.Sections.Containers.Items...)
	clone.Sections.Logs.Items = append([]any(nil), value.Sections.Logs.Items...)
	clone.Sections.Observer.Items = append([]any(nil), value.Sections.Observer.Items...)
	if value.Sections.Overview.Data != nil {
		data := *value.Sections.Overview.Data
		clone.Sections.Overview.Data = &data
	}
	clone.Sections.Overview.SectionStatus = cloneStatus(value.Sections.Overview.SectionStatus)
	clone.Sections.Filesystems.SectionStatus = cloneStatus(value.Sections.Filesystems.SectionStatus)
	clone.Sections.Processes.SectionStatus = cloneStatus(value.Sections.Processes.SectionStatus)
	clone.Sections.Services.SectionStatus = cloneStatus(value.Sections.Services.SectionStatus)
	clone.Sections.Containers.SectionStatus = cloneStatus(value.Sections.Containers.SectionStatus)
	clone.Sections.Logs.SectionStatus = cloneStatus(value.Sections.Logs.SectionStatus)
	clone.Sections.Observer.SectionStatus = cloneStatus(value.Sections.Observer.SectionStatus)
	return clone
}

func cloneStatus(value projection.SectionStatus) projection.SectionStatus {
	clone := value
	if value.ObservedAt != nil {
		at := *value.ObservedAt
		clone.ObservedAt = &at
	}
	if value.ReasonCode != nil {
		reason := *value.ReasonCode
		clone.ReasonCode = &reason
	}
	return clone
}
func previousSnapshot(raw observation.Snapshot) *observation.Snapshot {
	result := &observation.Snapshot{ObservedAt: raw.ObservedAt, Network: raw.Network}
	if raw.Network.Data != nil {
		data := *raw.Network.Data
		result.Network.Data = &data
	}
	return result
}
