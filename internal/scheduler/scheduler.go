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
	System            projection.SystemInfo
	Clock             Clock
	NewTicker         func(time.Duration) Ticker
}

func DefaultConfig() Config {
	return Config{Interval: 15 * time.Second, CollectionTimeout: 10 * time.Second, System: projection.NativeSystemInfo(), Clock: realClock{}, NewTicker: func(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }}
}

type Stats struct{ Collections, CollectionFailures, StoreFailures, TriggersDropped uint64 }

type Scheduler struct {
	collect      func(context.Context) observation.Snapshot
	store        Store
	config       Config
	trigger      chan struct{}
	started      atomic.Bool
	lifecycleMu  sync.Mutex
	cancel       context.CancelFunc
	done         chan struct{}
	mu           sync.RWMutex
	current      projection.CurrentSnapshot
	hasCurrent   bool
	previous     *observation.Snapshot
	lastSequence uint64
	stats        Stats
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
	ctx, cancel := context.WithTimeout(parent, s.config.CollectionTimeout)
	defer cancel()
	raw := s.collect(ctx)
	sequence, err := s.store.NextSequence(ctx)
	s.mu.Lock()
	if err != nil {
		s.stats.StoreFailures++
		s.lastSequence++
	} else {
		s.lastSequence = sequence
	}
	sequence = s.lastSequence
	s.stats.Collections++
	if raw.Quality.State == observation.Failed {
		s.stats.CollectionFailures++
	}
	s.mu.Unlock()
	current := projection.Current(raw, s.config.System)
	current.Sequence = sequence
	s.mu.Lock()
	s.current = current
	s.hasCurrent = true
	s.mu.Unlock()
	samples := history.Extract(raw, s.previous)
	if err := s.store.WriteSamples(ctx, samples); err != nil {
		s.mu.Lock()
		s.stats.StoreFailures++
		s.mu.Unlock()
	}
	if err := s.store.Maintain(ctx, s.config.Clock.Now()); err != nil {
		s.mu.Lock()
		s.stats.StoreFailures++
		s.mu.Unlock()
	}
	copy := raw
	s.previous = &copy
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
	return s.current, s.hasCurrent
}
func (s *Scheduler) Stats() Stats                { s.mu.RLock(); defer s.mu.RUnlock(); return s.stats }
func (s *Scheduler) StoreHealth() history.Health { return s.store.Health() }
