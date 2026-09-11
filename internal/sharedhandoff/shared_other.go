//go:build !linux

package sharedhandoff

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

type Writer struct{}
type Reader struct{}

func OpenWriter(string, Policy) (*Writer, error)                              { return nil, ErrUnsupported }
func OpenReader(string, Policy) (*Reader, error)                              { return nil, ErrUnsupported }
func (*Writer) Publish(observation.Snapshot, remoteprojection.Identity) error { return ErrUnsupported }
func (*Reader) Read(context.Context, string) ([]byte, error)                  { return nil, ErrUnsupported }
func (*Writer) Close() error                                                  { return nil }
func (*Reader) Close() error                                                  { return nil }
