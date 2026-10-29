package dockerreader

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func TestCycleKeepsOriginalObservationAndClosesBeforePublication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now := time.Date(2026, 10, 30, 0, 0, 5, 0, time.UTC)
	observed := now.Add(-time.Second)
	var order []string
	p := ports{
		guard: func() error { order = append(order, "guard"); return nil },
		collect: func(c context.Context) model.Inventory {
			order = append(order, "collect")
			d, ok := c.Deadline()
			if !ok || time.Until(d) > deadline || time.Until(d) < deadline-time.Second {
				t.Fatal("missing bounded acquisition deadline")
			}
			return model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &observed, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, Items: []model.Container{}}
		},
		close: func() error { order = append(order, "close"); return nil },
		publish: func(s workloadprojection.ContainerSection, at time.Time) error {
			order = append(order, "publish")
			stamp, ok := s.ObservedAt()
			if !ok || !stamp.Equal(observed) || !at.Equal(now) {
				t.Fatal("sample retimestamped or publication time lost")
			}
			return nil
		},
		status: func(r connectedstatus.Record) error {
			order = append(order, "status")
			if r.State != "COLLECTING" || !r.CollectedAt.Equal(observed) || r.InvocationID != "test" {
				t.Fatal("incorrect operational state")
			}
			return nil
		},
		now: func() time.Time { return now },
		wait: func(c context.Context, d time.Duration) error {
			if d != 15*time.Second {
				t.Fatal("unbounded cycle cadence")
			}
			cancel()
			return c.Err()
		},
	}
	if err := run(ctx, "test", p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"guard", "collect", "close", "guard", "publish", "status"}) {
		t.Fatal(order)
	}
}

func TestCycleFailuresNeverPublishUnauthorizedOrRawData(t *testing.T) {
	for _, fault := range []string{"pre_guard", "post_guard", "close", "cancel", "projection", "source", "publish", "status"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			guards, closes, publishes := 0, 0, 0
			p := ports{
				guard: func() error {
					guards++
					if fault == "pre_guard" || fault == "post_guard" && guards == 2 {
						return errors.New("private error")
					}
					return nil
				},
				collect: func(context.Context) model.Inventory {
					if fault == "cancel" {
						cancel()
					}
					if fault == "source" {
						reason := "DOCKER_UNAVAILABLE"
						return model.Inventory{SchemaVersion: model.SchemaVersion, SupportState: model.SupportUnavailable, CollectionState: model.CollectionNotRun, Freshness: model.FreshnessUnknown, ReasonCode: &reason}
					}
					return model.Inventory{} // Invalid raw data is never forwarded.
				},
				close: func() error {
					closes++
					if fault == "close" {
						return errors.New("private")
					}
					return nil
				},
				publish: func(s workloadprojection.ContainerSection, _ time.Time) error {
					publishes++
					if _, ok := s.ObservedAt(); ok {
						t.Fatal("unavailable source given a fresh observation")
					}
					if _, err := s.Bytes(); err != nil {
						t.Fatal("invalid handoff")
					}
					if fault == "publish" {
						return errors.New("private")
					}
					return nil
				},
				status: func(r connectedstatus.Record) error {
					want := "INVALID_SNAPSHOT"
					if fault == "source" {
						want = "SOURCE_UNAVAILABLE"
					}
					if r.State != want {
						t.Fatalf("state=%s", r.State)
					}
					if fault == "status" {
						return errors.New("private")
					}
					return nil
				},
				now:  time.Now,
				wait: func(c context.Context, _ time.Duration) error { cancel(); return c.Err() },
			}
			err := run(ctx, "test", p)
			fatal := fault == "pre_guard" || fault == "post_guard" || fault == "close" || fault == "publish" || fault == "status"
			if fatal && err != ErrUnsafe || !fatal && err != nil {
				t.Fatalf("result=%v", err)
			}
			if fault == "pre_guard" {
				if closes != 0 {
					t.Fatal("Engine touched before guard")
				}
			} else if closes != 1 {
				t.Fatal("acquisition not closed")
			}
			denied := fault == "pre_guard" || fault == "post_guard" || fault == "close" || fault == "cancel"
			if denied && publishes != 0 || !denied && publishes != 1 {
				t.Fatalf("publications=%d", publishes)
			}
		})
	}
}

func TestCyclePrecancelledAndIncompletePorts(t *testing.T) {
	if run(context.Background(), "", ports{}) != ErrUnsafe {
		t.Fatal("missing dependencies accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if wait(ctx, time.Hour) != context.Canceled {
		t.Fatal("cancel ignored")
	}
}
