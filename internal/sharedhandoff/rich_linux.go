//go:build linux

package sharedhandoff

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

type RichProcessesWriter struct{ s *slot }
type RichProcessesReader struct{ s *slot }

func (s *slot) maxBytes() int64 {
	if s.rich != nil {
		return RichProcessesMaxBytes
	}
	return remoteprojection.MaxBytes
}

func openRich(path string, p RichProcessesPolicy, writer bool) (*slot, error) {
	if !validRichPolicy(p) {
		return nil, ErrUnsafe
	}
	return openWithRich(path, p.Policy, writer, &p)
}

func OpenRichProcessesWriter(path string, p RichProcessesPolicy) (*RichProcessesWriter, error) {
	s, err := openRich(path, p, true)
	if err != nil {
		return nil, err
	}
	return &RichProcessesWriter{s}, nil
}

func OpenRichProcessesReader(path string, p RichProcessesPolicy) (*RichProcessesReader, error) {
	s, err := openRich(path, p, false)
	if err != nil {
		return nil, err
	}
	return &RichProcessesReader{s}, nil
}

func (w *RichProcessesWriter) Publish(body []byte) error {
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.lock == nil {
		return ErrUnavailable
	}
	if identity(s.policy, true) != nil || s.rich == nil {
		return ErrUnsafe
	}
	record, err := encodeRichProcesses(body, *s.rich)
	if err != nil {
		return ErrInvalid
	}
	return s.publishData(record)
}

func (r *RichProcessesReader) Read(ctx context.Context, expected string) ([]byte, error) {
	return (&Reader{r.s}).Read(ctx, expected)
}
func (w *RichProcessesWriter) Close() error { return w.s.close() }
func (r *RichProcessesReader) Close() error { return r.s.close() }
