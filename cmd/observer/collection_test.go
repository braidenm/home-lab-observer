package main

import (
	"context"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

type testContainerCollection func(context.Context)

func (collect testContainerCollection) Collect(ctx context.Context) containerobs.Inventory {
	collect(ctx)
	return containerobs.Disabled()
}

func TestContainerCollectionRunsAlongsideHostAndJoinsBeforeReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	hostStarted := make(chan struct{})
	containerStarted := make(chan struct{})
	containerFinished := make(chan struct{})
	want := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	collect := collectWithContainers(func(ctx context.Context) observation.Snapshot {
		close(hostStarted)
		select {
		case <-containerStarted:
		case <-ctx.Done():
			t.Error("container collection did not start concurrently")
		}
		return observation.Snapshot{ObservedAt: want}
	}, testContainerCollection(func(ctx context.Context) {
		close(containerStarted)
		select {
		case <-hostStarted:
		case <-ctx.Done():
		}
		close(containerFinished)
	}))
	if got := collect(ctx); got.ObservedAt != want {
		t.Fatal("container work changed host snapshot")
	}
	select {
	case <-containerFinished:
	default:
		t.Fatal("container work outlived collection")
	}
}
