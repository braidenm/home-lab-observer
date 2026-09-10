package journalreader

import (
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func (a *attempt) row() (*logobs.Event, time.Time, error) {
	at := a.request.QueryStartedAt
	micros, err := callValue(a, a.journal.RealtimeMicros)
	if err != nil {
		if errors.Is(err, ErrFieldMissing) {
			return nil, at, nil
		}
		return nil, at, err
	}
	if err := a.chargeNative(8); err != nil {
		return nil, at, err
	}
	nativeAt := time.Unix(int64(micros/1_000_000), int64(micros%1_000_000)*1000).UTC()
	if nativeAt.Year() < 1 || nativeAt.Year() > 9999 || nativeAt.IsZero() || nativeAt.After(a.request.QueryStartedAt.Add(sourceDeadline)) {
		return nil, at, nil
	}
	at = nativeAt
	priority, err := callValue(a, a.journal.Priority)
	if errors.Is(err, ErrFieldTooLarge) {
		return nil, at, a.chargeNative(logobs.MaxNativeFieldBytes)
	}
	if err != nil && !errors.Is(err, ErrFieldMissing) {
		return nil, at, err
	}
	if errors.Is(err, ErrFieldMissing) {
		priority = nil
	}
	if len(priority) > logobs.MaxNativeFieldBytes {
		return nil, at, a.chargeNative(logobs.MaxNativeFieldBytes)
	}
	if err := a.chargeNative(len(priority)); err != nil {
		return nil, at, err
	}
	priority = append([]byte(nil), priority...)
	messageID, err := callValue(a, a.journal.MessageID)
	if errors.Is(err, ErrFieldTooLarge) {
		return nil, at, a.chargeNative(logobs.MaxNativeFieldBytes)
	}
	if err != nil && !errors.Is(err, ErrFieldMissing) {
		return nil, at, err
	}
	if errors.Is(err, ErrFieldMissing) {
		messageID = nil
	}
	if len(messageID) > logobs.MaxNativeFieldBytes {
		return nil, at, a.chargeNative(logobs.MaxNativeFieldBytes)
	}
	if err := a.chargeNative(len(messageID)); err != nil {
		return nil, at, err
	}
	severity, validPriority := prioritySeverity(priority)
	code := ""
	if validMessageID(messageID) {
		code = "SYSTEMD_" + string(messageID)
	} else if validPriority {
		code = "SYSTEMD_PRIORITY_" + string(priority)
	}
	if code == "" {
		return nil, at, nil
	}
	event := &logobs.Event{ObservedAt: at, Source: logobs.SourceSystem, Severity: severity, EventCode: code}
	if event.Validate() != nil {
		return nil, at, nil
	}
	return event, time.Time{}, nil
}

func prioritySeverity(priority []byte) (logobs.Severity, bool) {
	if len(priority) != 1 {
		return logobs.SeverityUnknown, false
	}
	switch priority[0] {
	case '0', '1', '2':
		return logobs.SeverityCritical, true
	case '3':
		return logobs.SeverityError, true
	case '4':
		return logobs.SeverityWarn, true
	case '5', '6':
		return logobs.SeverityInfo, true
	case '7':
		return logobs.SeverityDebug, true
	default:
		return logobs.SeverityUnknown, false
	}
}
func validMessageID(value []byte) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
