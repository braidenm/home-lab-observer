package main

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

type containerCollection interface {
	Collect(context.Context) containerobs.Inventory
}

// The scheduler remains the single-flight owner; container reads never run on an HTTP request.
func collectWithContainers(host collectFunc, containers containerCollection) collectFunc {
	return func(ctx context.Context) observation.Snapshot {
		done := make(chan struct{})
		go func() { defer close(done); containers.Collect(ctx) }()
		current := host(ctx)
		<-done
		return current
	}
}
