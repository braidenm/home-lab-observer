package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/projection"
)

type fakeTicker struct {
	channel chan time.Time
	stopped atomic.Bool
}

func (t *fakeTicker) C() <-chan time.Time { return t.channel }
func (t *fakeTicker) Stop()               { t.stopped.Store(true) }

type fixedSchedulerClock struct{ at time.Time }

func (c fixedSchedulerClock) Now() time.Time { return c.at }

type fakeStore struct {
	mu                 sync.Mutex
	sequence           uint64
	writes             [][]history.Sample
	maintains          int
	closed             bool
	health             history.Health
	nextErr            error
	requireLiveContext bool
}

func (s *fakeStore) NextSequence(ctx context.Context) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.requireLiveContext && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if s.nextErr != nil {
		return 0, s.nextErr
	}
	s.sequence++
	return s.sequence, nil
}
func (s *fakeStore) WriteSamples(_ context.Context, v []history.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, v)
	return nil
}
func (s *fakeStore) Maintain(context.Context, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maintains++
	return nil
}
func (s *fakeStore) Health() history.Health { return s.health }
func (s *fakeStore) Close() error           { s.mu.Lock(); defer s.mu.Unlock(); s.closed = true; return nil }

func completeSnapshot(at time.Time) observation.Snapshot {
	cpu := observation.CPU{LogicalCPUs: 4, UsagePercent: 10}
	memory := observation.Memory{UsagePercent: 20}
	uptime := observation.Uptime{Seconds: 30}
	filesystems := []observation.Filesystem{}
	processes := []observation.Process{{PID: 7, Name: "original", State: "running"}}
	return observation.Snapshot{ObservedAt: at, Quality: observation.Quality{State: observation.Complete}, CPU: observation.Section[observation.CPU]{State: observation.Available, Data: &cpu}, Memory: observation.Section[observation.Memory]{State: observation.Available, Data: &memory}, Uptime: observation.Section[observation.Uptime]{State: observation.Available, Data: &uptime}, Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &filesystems}, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes}}
}

func TestImmediateCollectionMonotonicSequenceAndGracefulStop(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time, 1)}
	store := &fakeStore{sequence: 40}
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	config := Config{Interval: 15 * time.Second, CollectionTimeout: time.Second, System: projection.SystemInfo{OS: "linux", Architecture: "amd64"}, Clock: fixedSchedulerClock{at}, NewTicker: func(time.Duration) Ticker { return ticker }}
	scheduler, err := New(func(context.Context) observation.Snapshot { return completeSnapshot(at) }, store, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { snapshot, ok := scheduler.Current(); return ok && snapshot.Sequence == 41 })
	ticker.channel <- at.Add(15 * time.Second)
	waitFor(t, func() bool { snapshot, _ := scheduler.Current(); return snapshot.Sequence == 42 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	closed := store.closed
	maintains := store.maintains
	store.mu.Unlock()
	if !closed || maintains != 2 || !ticker.stopped.Load() {
		t.Fatalf("closed=%v maintains=%d tickerStopped=%v", closed, maintains, ticker.stopped.Load())
	}
}

func TestSingleFlightCoalescesTriggers(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time)}
	store := &fakeStore{}
	entered := make(chan struct{}, 3)
	release := make(chan struct{}, 3)
	var active, maxActive atomic.Int32
	collect := func(context.Context) observation.Snapshot {
		now := active.Add(1)
		for {
			old := maxActive.Load()
			if now <= old || maxActive.CompareAndSwap(old, now) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return completeSnapshot(time.Now().UTC())
	}
	config := Config{Interval: time.Hour, CollectionTimeout: time.Minute, System: projection.SystemInfo{OS: "linux", Architecture: "amd64"}, NewTicker: func(time.Duration) Ticker { return ticker }}
	scheduler, err := New(collect, store, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-entered
	if !scheduler.Trigger() {
		t.Fatal("first trigger should queue")
	}
	if scheduler.Trigger() {
		t.Fatal("second trigger should be coalesced")
	}
	release <- struct{}{}
	<-entered
	if maxActive.Load() != 1 {
		t.Fatalf("max active=%d", maxActive.Load())
	}
	release <- struct{}{}
	waitFor(t, func() bool { return scheduler.Stats().Collections == 2 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if scheduler.Stats().TriggersDropped != 1 {
		t.Fatalf("stats=%+v", scheduler.Stats())
	}
}

func TestConcurrentStartKeepsTheWinningCancellationFunction(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time)}
	store := &fakeStore{}
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	scheduler, err := New(
		func(context.Context) observation.Snapshot { return completeSnapshot(at) },
		store,
		Config{Interval: time.Hour, CollectionTimeout: time.Second, NewTicker: func(time.Duration) Ticker { return ticker }},
	)
	if err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int32
	var attempts sync.WaitGroup
	for range 16 {
		attempts.Add(1)
		go func() {
			defer attempts.Done()
			if scheduler.Start(context.Background()) == nil {
				successes.Add(1)
			}
		}()
	}
	attempts.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful starts=%d", successes.Load())
	}
	waitFor(t, func() bool { _, ok := scheduler.Current(); return ok })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatalf("winning scheduler was not canceled: %v", err)
	}
}

func TestCurrentReturnsDeepCopy(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time)}
	store := &fakeStore{}
	at := time.Now().UTC()
	scheduler, _ := New(func(context.Context) observation.Snapshot { return completeSnapshot(at) }, store, Config{Interval: time.Hour, NewTicker: func(time.Duration) Ticker { return ticker }})
	_ = scheduler.Start(context.Background())
	waitFor(t, func() bool { _, ok := scheduler.Current(); return ok })
	first, _ := scheduler.Current()
	first.Privacy.ExcludedFields[0] = "mutated"
	first.Sections.Processes.Items[0].Name = "mutated"
	*first.Sections.Processes.ObservedAt = time.Time{}
	second, _ := scheduler.Current()
	if second.Privacy.ExcludedFields[0] == "mutated" || second.Sections.Processes.Items[0].Name == "mutated" || second.Sections.Processes.ObservedAt.IsZero() {
		t.Fatal("caller mutated cached snapshot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = scheduler.Stop(ctx)
}

func TestFailedCollectionRetainsUsefulCurrent(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time, 1)}
	store := &fakeStore{}
	at := time.Now().UTC()
	var calls atomic.Int32
	collect := func(context.Context) observation.Snapshot {
		if calls.Add(1) == 1 {
			return completeSnapshot(at)
		}
		return observation.Snapshot{ObservedAt: at.Add(time.Second), Quality: observation.Quality{State: observation.Failed}}
	}
	scheduler, _ := New(collect, store, Config{Interval: time.Hour, NewTicker: func(time.Duration) Ticker { return ticker }})
	_ = scheduler.Start(context.Background())
	waitFor(t, func() bool { snapshot, ok := scheduler.Current(); return ok && snapshot.Sequence == 1 })
	ticker.channel <- at
	waitFor(t, func() bool { return scheduler.Stats().Collections == 2 })
	current, _ := scheduler.Current()
	if current.Sequence != 1 || current.CollectionState == "FAILED" {
		t.Fatalf("current=%+v", current)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = scheduler.Stop(ctx)
}

func TestSequenceFailureDoesNotPublishRegressedFallback(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time, 1)}
	store := &fakeStore{sequence: 99}
	at := time.Now().UTC()
	scheduler, _ := New(func(context.Context) observation.Snapshot { return completeSnapshot(at) }, store, Config{Interval: time.Hour, NewTicker: func(time.Duration) Ticker { return ticker }})
	_ = scheduler.Start(context.Background())
	waitFor(t, func() bool { snapshot, ok := scheduler.Current(); return ok && snapshot.Sequence == 100 })
	store.mu.Lock()
	store.nextErr = errors.New("durable sequence unavailable")
	store.mu.Unlock()
	ticker.channel <- at
	waitFor(t, func() bool { return scheduler.Stats().StoreFailures == 1 })
	current, _ := scheduler.Current()
	if current.Sequence != 100 {
		t.Fatalf("regressed sequence=%d", current.Sequence)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = scheduler.Stop(ctx)
}

func TestStorageGetsFreshContextAfterCollectionDeadline(t *testing.T) {
	ticker := &fakeTicker{channel: make(chan time.Time)}
	store := &fakeStore{requireLiveContext: true}
	at := time.Now().UTC()
	collect := func(ctx context.Context) observation.Snapshot { <-ctx.Done(); return completeSnapshot(at) }
	scheduler, _ := New(collect, store, Config{Interval: time.Hour, CollectionTimeout: 10 * time.Millisecond, StorageTimeout: time.Second, NewTicker: func(time.Duration) Ticker { return ticker }})
	_ = scheduler.Start(context.Background())
	waitFor(t, func() bool { _, ok := scheduler.Current(); return ok })
	if scheduler.Stats().StoreFailures != 0 {
		t.Fatalf("stats=%+v", scheduler.Stats())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = scheduler.Stop(ctx)
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}
