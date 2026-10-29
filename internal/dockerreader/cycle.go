// Package dockerreader contains the installed reader's bounded sampling loop.
// Acquisition stays behind the existing local Docker adapter; only a projected
// container section crosses the handoff boundary, never an Engine response.
package dockerreader

import (
	"context"
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var ErrUnsafe = errors.New("docker_reader_refused")

const interval = 15 * time.Second
const deadline = 4 * time.Second

// These private ports are a test seam, not a configurable endpoint or plugin API.
type ports struct {
	guard   func() error
	collect func(context.Context) model.Inventory
	close   func() error
	publish func(workloadprojection.ContainerSection, time.Time) error
	status  func(connectedstatus.Record) error
	now     func() time.Time
	wait    func(context.Context, time.Duration) error
}

func run(ctx context.Context, invocation string, p ports) error {
	if ctx == nil || p.guard == nil || p.collect == nil || p.close == nil || p.publish == nil || p.status == nil || p.now == nil || p.wait == nil {
		return ErrUnsafe
	}
	for ctx.Err() == nil {
		if p.guard() != nil {
			return ErrUnsafe
		}
		cycle, cancel := context.WithTimeout(ctx, deadline)
		inventory := p.collect(cycle)
		cancel()
		// Keep one adapter (and its CPU sample baseline), but no idle Engine
		// connection across cycles. Acquisition has already joined all workers.
		if p.close() != nil {
			return ErrUnsafe
		}
		if ctx.Err() != nil {
			return nil
		}
		if p.guard() != nil {
			return ErrUnsafe
		}
		section, err := workloadprojection.ProjectContainers(inventory)
		state := "COLLECTING"
		if inventory.SupportState != model.SupportSupported {
			state = "SOURCE_UNAVAILABLE"
		}
		if err != nil {
			section = workloadprojection.UnavailableContainers()
			state = "INVALID_SNAPSHOT"
		}
		now := p.now().UTC()
		if p.publish(section, now) != nil {
			return ErrUnsafe
		}
		observed, _ := section.ObservedAt()
		if p.status(connectedstatus.Record{Version: "observer-connected-status/v1", InvocationID: invocation, State: state, UpdatedAt: now, CollectedAt: observed}) != nil {
			return ErrUnsafe
		}
		if p.wait(ctx, interval) != nil {
			if ctx.Err() != nil {
				return nil
			}
			return ErrUnsafe
		}
	}
	return nil
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
