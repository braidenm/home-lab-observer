package eventreader

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const sourceDeadline = 2 * time.Second
const maxNativeVisitBytes = logobs.MaxCheckpointBytes + 4*logobs.MaxNativeFieldBytes
const fileTimeEpochOffset int64 = 11644473600

var errNativeBudget = errors.New("EVENT_LOG_NATIVE_METADATA_TOO_LARGE")

type attempt struct {
	ctx         context.Context
	request     logobs.ReadRequest
	factory     Factory
	query       Query
	batch       logobs.Batch
	supported   bool
	nativeBytes int
}

// Read is thread-affine and cooperatively bounded. The future parent helper
// adapter must also enforce the deadline and kill/reap blocked native work.
func (r *Reader) Read(parent context.Context, request logobs.ReadRequest) (logobs.Batch, error) {
	if parent == nil || request.Validate() != nil {
		return logobs.Batch{}, ErrInvalidRequest
	}
	request = request.Clone()
	var lower uint64
	if !request.Checkpoint.ResetPending && len(request.Checkpoint.Opaque) == 0 {
		at := request.QueryStartedAt.Add(-5 * time.Minute)
		seconds := at.Unix() + fileTimeEpochOffset
		if seconds < 0 {
			return logobs.Batch{}, ErrInvalidRequest
		}
		lower = uint64(seconds)*10000000 + uint64((at.Nanosecond()+99)/100)
	}
	started := r.now().UTC()
	if started.Before(request.QueryStartedAt) {
		return logobs.Batch{}, ErrReadFailed
	}
	ctx, cancel := context.WithTimeout(parent, sourceDeadline)
	defer cancel()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := attempt{ctx: ctx, request: request, factory: r.factory, batch: logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision, QueryStartedAt: request.QueryStartedAt, StartedAt: started}}
	defer a.closeQuery()
	if request.Checkpoint.ResetPending {
		a.batch.Kind = logobs.BatchResetPending
	}
	err := a.run(lower)
	a.closeQuery()
	if err == nil {
		err = ctx.Err()
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

func (a *attempt) closeQuery() {
	if a.query != nil {
		a.query.Close()
		a.query = nil
	}
}
func (a *attempt) open(create func() (Query, error)) error {
	a.closeQuery()
	q, err := callValue(a, create)
	if q != nil {
		a.query = q
	}
	if err != nil {
		return err
	}
	if q == nil {
		return ErrReadFailed
	}
	a.supported = true
	return nil
}
func (a *attempt) run(lower uint64) error {
	if a.request.Checkpoint.ResetPending {
		return a.tail(true)
	}
	if len(a.request.Checkpoint.Opaque) == 0 {
		if err := a.open(func() (Query, error) { return a.factory.OpenInitial(a.request.Source, lower) }); err != nil {
			return err
		}
	} else {
		if err := a.open(func() (Query, error) { return a.factory.OpenContinuation(a.request.Source) }); err != nil {
			return err
		}
		_, err := callValue(a, func() (bool, error) { return true, a.query.SeekBookmark(a.request.Checkpoint.Opaque) })
		if errors.Is(err, ErrBookmarkStale) {
			return a.tail(true)
		}
		if err != nil {
			return err
		}
		cursor, exact, err := a.probe(true, nil)
		if err != nil {
			return err
		}
		if !exact {
			return a.tail(true)
		}
		a.batch.NextOpaque = cursor
	}
	return a.ingest()
}

func (a *attempt) next() (Record, error) {
	r, err := callValue(a, a.query.Next)
	if err != nil && r != nil {
		r.Close()
		r = nil
	}
	return r, err
}

// probe always closes its record before the caller can open another query.
func (a *attempt) probe(verify bool, before *time.Time) ([]byte, bool, error) {
	reservation := logobs.MaxCheckpointBytes
	if before != nil {
		reservation += logobs.MaxNativeFieldBytes
	}
	if err := a.reserve(reservation); err != nil {
		return nil, false, err
	}
	r, err := a.next()
	if err != nil {
		return nil, false, err
	}
	if r == nil {
		if verify {
			return nil, false, errors.New("EVENT_LOG_PROBE_NOT_PROVEN")
		}
		return nil, false, nil
	}
	defer r.Close()
	a.batch.ExaminedCount++
	a.batch.ProbeCount++
	cursor, err := a.bookmark(r)
	if err != nil {
		return nil, false, err
	}
	if verify {
		exact, err := callValue(a, func() (bool, error) { return r.BookmarkMatches(a.request.Checkpoint.Opaque) })
		return cursor, exact, err
	}
	if before != nil {
		ticks, valid, err := selected(a, r.TimeCreated, 8)
		if err != nil && !errors.Is(err, errDiscardRow) {
			return nil, false, err
		}
		at := fileTimeUTC(ticks)
		if err != nil || !valid || at.Year() > 9999 || !at.Before(*before) {
			return nil, false, errors.New("EVENT_LOG_INITIAL_TAIL_NOT_PROVEN")
		}
	}
	return cursor, true, nil
}

func (a *attempt) tail(reset bool) error {
	if reset {
		a.batch.Kind = logobs.BatchResetPending
	}
	if err := a.open(func() (Query, error) { return a.factory.OpenTail(a.request.Source) }); err != nil {
		return err
	}
	var before *time.Time
	if !reset {
		lower := a.request.QueryStartedAt.Add(-5 * time.Minute)
		before = &lower
	}
	cursor, visible, err := a.probe(false, before)
	if err != nil {
		return err
	}
	if !visible {
		if reset {
			a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonCheckpointReset)
		} else {
			a.setState(logobs.SupportUnavailable, logobs.CollectionNotRun, logobs.ReasonNoVisibleJournal)
		}
		return nil
	}
	a.batch.NextOpaque = cursor
	if reset {
		a.batch.Kind = logobs.BatchResetEstablished
		a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonCheckpointReset)
	} else {
		a.caughtUp()
	}
	return nil
}

func (a *attempt) ingest() error {
	maximum := logobs.MaxAcceptedEvents - int(a.batch.ProbeCount)
	for processed := 0; ; processed++ {
		if err := a.reserve(maxNativeVisitBytes); err != nil {
			if processed == 0 {
				return err
			}
			a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonResponseTooLarge)
			return nil
		}
		r, err := a.next()
		if err != nil {
			return err
		}
		if r == nil {
			if processed == 0 && a.batch.ProbeCount == 0 {
				return a.tail(false)
			}
			a.caughtUp()
			return nil
		}
		stop, err := a.ingestRecord(r, processed == maximum)
		if err != nil || stop {
			return err
		}
	}
}
func (a *attempt) ingestRecord(r Record, sentinel bool) (bool, error) {
	defer r.Close()
	a.batch.ExaminedCount++
	cursor, err := a.bookmark(r)
	if err != nil {
		return false, err
	}
	if sentinel {
		a.batch.Deferred = true
		a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonBacklogDeferred)
		return true, nil
	}
	event, discardAt, err := a.row(r)
	if err != nil {
		return false, err
	}
	if event != nil {
		a.batch.Events = append(a.batch.Events, *event)
	} else {
		a.batch.Discards = append(a.batch.Discards, logobs.DiscardCount{At: discardAt, Count: 1})
		a.batch.DiscardedCount++
	}
	a.batch.NextOpaque = cursor
	return false, nil
}
func (a *attempt) bookmark(r Record) ([]byte, error) {
	value, err := callValue(a, r.Bookmark)
	if err != nil || len(value) == 0 || len(value) > logobs.MaxCheckpointBytes {
		return nil, ErrReadFailed
	}
	if err := a.charge(len(value)); err != nil {
		return nil, err
	}
	return append([]byte(nil), value...), nil
}
func (a *attempt) reserve(size int) error {
	if size < 0 || size > logobs.MaxSourceBytes-a.nativeBytes {
		return errNativeBudget
	}
	return nil
}
func (a *attempt) charge(size int) error {
	if err := a.reserve(size); err != nil {
		return err
	}
	a.nativeBytes += size
	return nil
}
func (a *attempt) caughtUp() {
	a.batch.CaughtUp = true
	if a.batch.DiscardedCount > 0 {
		a.setState(logobs.SupportSupported, logobs.CollectionPartial, logobs.ReasonInvalidResponse)
	} else {
		a.setState(logobs.SupportSupported, logobs.CollectionOK, "")
	}
}
func (a *attempt) setState(s logobs.SupportState, c logobs.CollectionState, reason logobs.ReasonCode) {
	a.batch.SupportState = s
	a.batch.CollectionState = c
	a.batch.ReasonCode = nil
	if reason != "" {
		a.batch.ReasonCode = &reason
	}
}
func (a *attempt) fail(err error) {
	kind := a.batch.Kind
	if kind == logobs.BatchResetEstablished {
		kind = logobs.BatchResetPending
	}
	a.batch = logobs.Batch{Kind: kind, Source: a.request.Source, ExpectedRevision: a.request.Checkpoint.Revision, QueryStartedAt: a.request.QueryStartedAt, StartedAt: a.batch.StartedAt}
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
func callValue[T any](a *attempt, action func() (T, error)) (T, error) {
	var zero T
	if err := a.ctx.Err(); err != nil {
		return zero, err
	}
	value, err := action()
	if stopped := a.ctx.Err(); stopped != nil {
		return value, stopped
	}
	return value, err
}
