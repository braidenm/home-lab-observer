//go:build linux

package actionjournal

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// AdmissionSnapshot never calls Acquire or performs a transition. The existing
// exclusive store handle, validated bounded loader and mutex own both streams.
func (j *Journal) AdmissionSnapshot(ctx context.Context) (AdmissionSnapshot, error) {
	if j == nil || ctx == nil || ctx.Err() != nil {
		return AdmissionSnapshot{}, ErrRecovery
	}
	if !j.mu.TryLock() {
		return AdmissionSnapshot{}, ErrBusy
	}
	defer j.mu.Unlock()
	if !j.usable() {
		return AdmissionSnapshot{}, ErrRecovery
	}
	bounded, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	records, err := j.records(bounded)
	if err != nil || bounded.Err() != nil {
		return AdmissionSnapshot{}, ErrRecovery
	}
	snapshot := AdmissionSnapshot{Binding: j.binding, ObservedAt: j.now().UTC(), Records: make([]AdmissionRecord, 0, len(records))}
	for _, r := range records {
		code := r.code
		if !r.intent.IsZero() && r.terminal.IsZero() {
			code = actioncore.OutcomeUnknown
		}
		_, active := j.active[r.key.ActionID]
		snapshot.Records = append(snapshot.Records, AdmissionRecord{r.key, !r.intent.IsZero(), code, r.terminal, active})
	}
	if bounded.Err() != nil {
		return AdmissionSnapshot{}, ErrRecovery
	}
	return snapshot, nil
}
