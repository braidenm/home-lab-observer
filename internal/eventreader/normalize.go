package eventreader

import (
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var errDiscardRow = errors.New("EVENT_LOG_DISCARD_SELECTED_ROW")

func selected[T any](a *attempt, read func() (T, error), size int) (T, bool, error) {
	value, err := callValue(a, read)
	var zero T
	if errors.Is(err, ErrFieldMissing) {
		return zero, false, nil
	}
	if errors.Is(err, ErrFieldInvalid) || errors.Is(err, ErrFieldTooLarge) {
		if chargeErr := a.charge(logobs.MaxNativeFieldBytes); chargeErr != nil {
			return zero, false, chargeErr
		}
		if errors.Is(err, ErrFieldTooLarge) {
			return zero, false, errDiscardRow
		}
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	if err := a.charge(size); err != nil {
		return zero, false, err
	}
	return value, true, nil
}

func (a *attempt) row(r Record) (event *logobs.Event, discardAt time.Time, err error) {
	discardAt = a.request.QueryStartedAt
	defer func() {
		if errors.Is(err, errDiscardRow) {
			err = nil
		}
	}()
	ticks, valid, err := selected(a, r.TimeCreated, 8)
	if err != nil || !valid {
		return nil, discardAt, err
	}
	at := fileTimeUTC(ticks)
	if at.IsZero() || at.Year() < 1 || at.Year() > 9999 || at.After(a.request.QueryStartedAt.Add(sourceDeadline)) {
		return nil, discardAt, nil
	}
	discardAt = at
	level, _, err := selected(a, r.Level, 1)
	if err != nil {
		return nil, discardAt, err
	}
	id, valid, err := selected(a, r.EventID, 2)
	if err != nil || !valid {
		return nil, discardAt, err
	}
	guid, hasGUID, err := selected(a, r.ProviderGUID, 16)
	if err != nil {
		return nil, discardAt, err
	}
	code := "WIN_"
	if hasGUID {
		code += hex.EncodeToString(guid[:]) + "_"
	}
	code += strconv.FormatUint(uint64(id), 10)
	event = &logobs.Event{Source: a.request.Source, ObservedAt: at, Severity: levelSeverity(level), EventCode: code}
	if event.Validate() != nil {
		return nil, discardAt, nil
	}
	return event, time.Time{}, nil
}

func fileTimeUTC(ticks uint64) time.Time {
	return time.Unix(int64(ticks/10000000)-fileTimeEpochOffset, int64(ticks%10000000)*100).UTC()
}
func levelSeverity(level uint8) logobs.Severity {
	switch level {
	case 1:
		return logobs.SeverityCritical
	case 2:
		return logobs.SeverityError
	case 3:
		return logobs.SeverityWarn
	case 4:
		return logobs.SeverityInfo
	case 5:
		return logobs.SeverityTrace
	default:
		return logobs.SeverityUnknown
	}
}
