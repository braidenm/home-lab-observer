package journalreader

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const sourceDeadline = 2 * time.Second

const maxNativeVisitBytes = logobs.MaxCheckpointBytes + 8 + 2*logobs.MaxNativeFieldBytes

var errNativeBudget = errors.New("JOURNAL_NATIVE_METADATA_TOO_LARGE")

type attempt struct {
	ctx         context.Context
	request     logobs.ReadRequest
	journal     Journal
	batch       logobs.Batch
	supported   bool
	nativeBytes int
}

// Read keeps native work on one OS thread. Context checks are cooperative; a
// separate process parent must still kill/reap a helper blocked in a native call.
func (r *Reader) Read(parent context.Context, request logobs.ReadRequest) (logobs.Batch, error) {
	if parent == nil || request.Validate() != nil || request.Source != logobs.SourceSystem {
		return logobs.Batch{}, ErrInvalidRequest
	}
	request = request.Clone()
	lower := request.QueryStartedAt.Add(-5 * time.Minute)
	if !request.Checkpoint.ResetPending && len(request.Checkpoint.Opaque) == 0 && lower.Unix() < 0 {
		return logobs.Batch{}, ErrInvalidRequest
	}
	started := r.now().UTC()
	if started.Before(request.QueryStartedAt) {
		return logobs.Batch{}, ErrReadFailed
	}
	ctx, cancel := context.WithTimeout(parent, sourceDeadline)
	defer cancel()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := attempt{ctx: ctx, request: request, batch: logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source,
		ExpectedRevision: request.Checkpoint.Revision, QueryStartedAt: request.QueryStartedAt, StartedAt: started}}
	if request.Checkpoint.ResetPending {
		a.batch.Kind = logobs.BatchResetPending
	}
	journal, err := callValue(&a, r.factory.OpenSystem)
	if journal != nil {
		defer journal.Close()
	}
	if err == nil && journal == nil {
		err = ErrReadFailed
	}
	if err == nil {
		a.journal, a.supported = journal, true
		err = a.run(lower)
	}
	if parent.Err() != nil {
		return logobs.Batch{}, parent.Err()
	}
	if errors.Is(err, ErrReadFailed) {
		return logobs.Batch{}, ErrReadFailed
	}
	if err != nil {
		a.fail(err)
	}
	a.batch.FinishedAt = r.now().UTC()
	if a.batch.Validate() != nil {
		return logobs.Batch{}, ErrReadFailed
	}
	return a.batch.Clone(), nil
}

func (a *attempt) run(lower time.Time) error {
	if a.request.Checkpoint.ResetPending {
		return a.tail(true, 0)
	}
	if len(a.request.Checkpoint.Opaque) != 0 {
		if err := call(a, func() error { return a.journal.SeekCursor(a.request.Checkpoint.Opaque) }); err != nil {
			if errors.Is(err, ErrInvalidCursor) {
				return a.tail(true, 0)
			}
			return err
		}
		if err := a.reserveNative(logobs.MaxCheckpointBytes); err != nil {
			return err
		}
		visited, err := callValue(a, a.journal.Next)
		if err != nil {
			return err
		}
		if !visited {
			return a.tail(true, 0)
		}
		a.batch.ExaminedCount++
		a.batch.ProbeCount++
		cursor, err := a.cursor()
		if err != nil {
			return err
		}
		exact, err := callValue(a, func() (bool, error) { return a.journal.TestCursor(a.request.Checkpoint.Opaque) })
		if err != nil && !errors.Is(err, ErrInvalidCursor) {
			return err
		}
		if !exact || err != nil {
			return a.tail(true, 0)
		}
		a.batch.NextOpaque = cursor
		return a.ingest(0)
	} else {
		micros := ceilRealtimeMicros(lower)
		if err := call(a, func() error { return a.journal.SeekRealtime(micros) }); err != nil {
			return err
		}
		return a.ingest(micros)
	}
}

func ceilRealtimeMicros(value time.Time) uint64 {
	return uint64(value.Unix())*1_000_000 + uint64((value.Nanosecond()+999)/1000)
}

func (a *attempt) tail(reset bool, initialLowerMicros uint64) error {
	if reset {
		a.batch.Kind = logobs.BatchResetPending
	}
	if err := call(a, a.journal.SeekTail); err != nil {
		return err
	}
	reserve := logobs.MaxCheckpointBytes
	if !reset {
		reserve += 8
	}
	if err := a.reserveNative(reserve); err != nil {
		return err
	}
	visited, err := callValue(a, a.journal.Previous)
	if err != nil {
		return err
	}
	if !visited {
		if reset {
			a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonCheckpointReset)
		} else {
			a.setState(logobs.SupportUnavailable, logobs.CollectionNotRun, logobs.ReasonNoVisibleJournal)
		}
		return nil
	}
	a.batch.ExaminedCount++
	a.batch.ProbeCount++
	cursor, err := a.cursor()
	if err != nil {
		return err
	}
	a.batch.NextOpaque = cursor
	if reset {
		a.batch.Kind = logobs.BatchResetEstablished
		a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonCheckpointReset)
	} else {
		micros, err := callValue(a, a.journal.RealtimeMicros)
		if err != nil {
			return err
		}
		if err := a.chargeNative(8); err != nil {
			return err
		}
		// A tail that arrived after the initial range read cannot prove the
		// queried window empty. Reject the whole attempt instead of advancing
		// a cursor past a concurrent record.
		if initialLowerMicros == 0 || micros >= initialLowerMicros {
			return ErrReadFailed
		}
		a.caughtUp()
	}
	return nil
}

func (a *attempt) ingest(initialLowerMicros uint64) error {
	maximum := logobs.MaxAcceptedEvents - int(a.batch.ProbeCount)
	processed := 0
	for {
		if err := a.reserveNative(maxNativeVisitBytes); err != nil {
			if processed == 0 {
				return err
			}
			a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonResponseTooLarge)
			return nil
		}
		visited, err := callValue(a, a.journal.Next)
		if err != nil {
			return err
		}
		if !visited {
			if processed == 0 && a.batch.ProbeCount == 0 {
				return a.tail(false, initialLowerMicros)
			}
			a.caughtUp()
			return nil
		}
		a.batch.ExaminedCount++
		cursor, err := a.cursor()
		if err != nil {
			return err
		}
		if processed == maximum {
			a.batch.Deferred = true
			a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonBacklogDeferred)
			return nil
		}
		event, discardAt, err := a.row()
		if err != nil {
			return err
		}
		if event != nil {
			a.batch.Events = append(a.batch.Events, *event)
		} else {
			a.batch.Discards = append(a.batch.Discards, logobs.DiscardCount{At: discardAt, Count: 1})
			a.batch.DiscardedCount++
		}
		a.batch.NextOpaque = cursor
		processed++
	}
}

func (a *attempt) cursor() ([]byte, error) {
	value, err := callValue(a, a.journal.Cursor)
	if err != nil || len(value) == 0 || len(value) > logobs.MaxCheckpointBytes {
		return nil, ErrReadFailed
	}
	if err := a.chargeNative(len(value)); err != nil {
		return nil, err
	}
	return append([]byte(nil), value...), nil
}

func (a *attempt) reserveNative(size int) error {
	if size < 0 || size > logobs.MaxSourceBytes-a.nativeBytes {
		return errNativeBudget
	}
	return nil
}

func (a *attempt) chargeNative(size int) error {
	if err := a.reserveNative(size); err != nil {
		return err
	}
	a.nativeBytes += size
	return nil
}

func (a *attempt) caughtUp() {
	a.batch.CaughtUp = true
	if a.batch.DiscardedCount != 0 {
		a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonInvalidResponse)
	} else {
		a.setState(logobs.SupportSupported, logobs.CollectionOK, "")
	}
}

func (a *attempt) setState(support logobs.SupportState, collection logobs.CollectionState, reason logobs.ReasonCode) {
	a.batch.SupportState, a.batch.CollectionState = support, collection
	a.batch.ReasonCode = nil
	if reason != "" {
		a.batch.ReasonCode = &reason
	}
}

func (a *attempt) fail(err error) {
	// Native failures reject any tentative prefix. A proved stale cursor keeps
	// reset pending, but no failed attempt advances an ingestion checkpoint.
	kind := a.batch.Kind
	if kind == logobs.BatchResetEstablished {
		kind = logobs.BatchResetPending
	}
	a.batch = logobs.Batch{Kind: kind, Source: a.request.Source, ExpectedRevision: a.request.Checkpoint.Revision,
		QueryStartedAt: a.request.QueryStartedAt, StartedAt: a.batch.StartedAt}
	switch {
	case errors.Is(err, ErrPermissionDenied):
		a.setState(logobs.SupportPermissionDenied, logobs.CollectionNotRun, logobs.ReasonPermissionDenied)
	case !a.supported && errors.Is(err, ErrUnavailable):
		a.setState(logobs.SupportUnavailable, logobs.CollectionNotRun, logobs.ReasonLogHelperUnavailable)
	case a.supported && errors.Is(err, context.DeadlineExceeded):
		a.setState(logobs.SupportSupported, logobs.CollectionFailed, logobs.ReasonDeadlineExceeded)
	case a.supported && errors.Is(err, errNativeBudget):
		a.setState(logobs.SupportSupported, logobs.CollectionFailed, logobs.ReasonResponseTooLarge)
	case a.supported:
		a.setState(logobs.SupportSupported, logobs.CollectionFailed, logobs.ReasonReaderFailed)
	default:
		a.setState(logobs.SupportUnavailable, logobs.CollectionFailed, logobs.ReasonReaderFailed)
	}
}

func call(a *attempt, action func() error) error {
	_, err := callValue(a, func() (bool, error) { return true, action() })
	return err
}
func callValue[T any](a *attempt, action func() (T, error)) (T, error) {
	var zero T
	if err := a.ctx.Err(); err != nil {
		return zero, err
	}
	value, err := action()
	if contextErr := a.ctx.Err(); contextErr != nil {
		return value, contextErr
	}
	return value, err
}
