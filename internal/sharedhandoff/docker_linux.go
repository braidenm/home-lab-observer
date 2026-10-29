//go:build linux

package sharedhandoff

import (
	"context"
	"time"

	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

type RichProcessesDockerWriter struct{ s *slot }
type RichProcessesDockerReader struct{ s *slot }
type DockerFrameWriter struct{ s *slot }
type DockerFrameReader struct{ s *slot }

func OpenRichProcessesDockerWriter(path string, p RichProcessesDockerPolicy) (*RichProcessesDockerWriter, error) {
	if !validDockerPolicy(p) {
		return nil, ErrUnsafe
	}
	s, err := openWithPolicies(path, p.Policy, true, nil, &p, nil)
	if err != nil {
		return nil, err
	}
	return &RichProcessesDockerWriter{s}, nil
}
func OpenRichProcessesDockerReader(path string, p RichProcessesDockerPolicy) (*RichProcessesDockerReader, error) {
	if !validDockerPolicy(p) {
		return nil, ErrUnsafe
	}
	s, err := openWithPolicies(path, p.Policy, false, nil, &p, nil)
	if err != nil {
		return nil, err
	}
	return &RichProcessesDockerReader{s}, nil
}
func OpenDockerFrameWriter(path string, p DockerFramePolicy) (*DockerFrameWriter, error) {
	if !validFramePolicy(p) {
		return nil, ErrUnsafe
	}
	s, err := openWithPolicies(path, p.Policy, true, nil, nil, &p)
	if err != nil {
		return nil, err
	}
	return &DockerFrameWriter{s}, nil
}
func OpenDockerFrameReader(path string, p DockerFramePolicy) (*DockerFrameReader, error) {
	if !validFramePolicy(p) {
		return nil, ErrUnsafe
	}
	s, err := openWithPolicies(path, p.Policy, false, nil, nil, &p)
	if err != nil {
		return nil, err
	}
	return &DockerFrameReader{s}, nil
}
func (w *RichProcessesDockerWriter) Publish(body []byte) error {
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.lock == nil {
		return ErrUnavailable
	}
	if identity(s.policy, true) != nil || s.richDocker == nil {
		return ErrUnsafe
	}
	record, err := encodeRichProcessesDocker(body, *s.richDocker)
	if err != nil {
		return ErrInvalid
	}
	return s.publishData(record)
}
func (w *DockerFrameWriter) Publish(section workloadprojection.ContainerSection, at time.Time) error {
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.lock == nil {
		return ErrUnavailable
	}
	if identity(s.policy, true) != nil || s.frame == nil {
		return ErrUnsafe
	}
	record, err := encodeDockerFrame(section, at, *s.frame)
	if err != nil {
		return ErrInvalid
	}
	return s.publishData(record)
}
func (r *RichProcessesDockerReader) Read(ctx context.Context, expected string) ([]byte, error) {
	return (&Reader{r.s}).Read(ctx, expected)
}
func (r *DockerFrameReader) Read(ctx context.Context, expected string, now time.Time) (workloadprojection.ContainerSection, error) {
	b, err := (&Reader{r.s}).Read(ctx, expected)
	if err != nil {
		return workloadprojection.ContainerSection{}, err
	}
	section, at, err := decodeDockerFrame(b, *r.s.frame)
	if err != nil || !freshDockerFrame(section, at, now) {
		return workloadprojection.ContainerSection{}, ErrInvalid
	}
	return section, nil
}
func (w *RichProcessesDockerWriter) Close() error { return w.s.close() }
func (r *RichProcessesDockerReader) Close() error { return r.s.close() }
func (w *DockerFrameWriter) Close() error         { return w.s.close() }
func (r *DockerFrameReader) Close() error         { return r.s.close() }
