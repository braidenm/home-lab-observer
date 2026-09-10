package logprotocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var fixtureTime = time.Date(2026, 9, 10, 12, 0, 0, 123, time.UTC)

func fixture() (Build, logobs.ReadRequest, logobs.Batch) {
	build := Build{Version: "0.1.0-preview.3", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64"}
	previous := fixtureTime.Add(-time.Minute)
	request := logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: fixtureTime,
		Checkpoint: logobs.Checkpoint{Revision: 7, Opaque: []byte("SYNTHETIC_PRIVATE_CHECKPOINT"), PreviousAttemptAt: &previous}}
	reason := logobs.ReasonInvalidResponse
	batch := logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
		QueryStartedAt: fixtureTime, StartedAt: fixtureTime, FinishedAt: fixtureTime.Add(time.Second),
		SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionPartial, ReasonCode: &reason,
		Events:        []logobs.Event{{ObservedAt: fixtureTime.Add(-time.Second), Source: request.Source, Severity: logobs.SeverityError, EventCode: "SYSTEMD_PRIORITY_3"}},
		Discards:      []logobs.DiscardCount{{At: fixtureTime, Count: 1}},
		ExaminedCount: 3, ProbeCount: 1, DiscardedCount: 1, CaughtUp: true, NextOpaque: []byte("SYNTHETIC_PRIVATE_NEXT_CURSOR")}
	return build, request, batch
}

func packets(t *testing.T) (Build, logobs.ReadRequest, logobs.Batch, []byte, []byte) {
	t.Helper()
	build, request, batch := fixture()
	req, err := EncodeRequest(build, request)
	if err != nil {
		t.Fatal("valid request encode failed")
	}
	resp, err := EncodeResponse(build, request, batch)
	if err != nil {
		t.Fatal("valid response encode failed")
	}
	return build, request, batch, req, resp
}

func TestRoundTripAndPrivatePrecision(t *testing.T) {
	for _, os := range []string{"linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			build, request, batch := fixture()
			build.OS, build.Arch = os, arch
			if os == "windows" {
				request.Source, batch.Source, batch.Events[0].Source = logobs.SourceApplication, logobs.SourceApplication, logobs.SourceApplication
				batch.Events[0].EventCode = "WIN_42"
			}
			request.Checkpoint.Revision, batch.ExpectedRevision = ^uint64(0), ^uint64(0)
			payload, err := EncodeRequest(build, request)
			if err != nil {
				t.Fatal("request encode failed")
			}
			decoded, err := DecodeRequest(payload, build)
			if err != nil || !reflect.DeepEqual(decoded, request) {
				t.Fatal("request round trip changed private values")
			}
			payload, err = EncodeResponse(build, request, batch)
			if err != nil {
				t.Fatal("response encode failed")
			}
			result, err := DecodeResponse(payload, build, request)
			if err != nil || !reflect.DeepEqual(result, batch) {
				t.Fatal("response round trip changed private values")
			}
			request.Checkpoint.Opaque[0] ^= 1
			batch.NextOpaque[0] ^= 1
			if bytes.Equal(decoded.Checkpoint.Opaque, request.Checkpoint.Opaque) || bytes.Equal(result.NextOpaque, batch.NextOpaque) {
				t.Fatal("decoded private bytes alias caller state")
			}
		}
	}
}

func TestClosedRequiredKeysAtEveryLevel(t *testing.T) {
	build, request, _, req, resp := packets(t)
	for _, tc := range []struct {
		name     string
		payload  []byte
		path     []any
		response bool
		nullable map[string]bool
	}{
		{"request", req, nil, false, nil},
		{"request-build", req, []any{"build"}, false, nil},
		{"checkpoint", req, []any{"checkpoint"}, false, map[string]bool{"opaque_base64": true, "previous_attempt_at": true, "coverage_through": true}},
		{"response", resp, nil, true, nil},
		{"response-build", resp, []any{"build"}, true, nil},
		{"batch", resp, []any{"batch"}, true, map[string]bool{"reason_code": true, "next_opaque_base64": true}},
		{"event", resp, []any{"batch", "events", 0}, true, nil},
		{"discard", resp, []any{"batch", "discards", 0}, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := objectAt(t, tc.payload, tc.path)
			for key := range original {
				for _, mode := range []string{"missing", "case", "null", "duplicate"} {
					if mode == "null" && tc.nullable[key] {
						continue
					}
					candidate := changeObject(t, tc.payload, tc.path, func(object map[string]any) {
						switch mode {
						case "missing":
							delete(object, key)
						case "case":
							object[strings.ToUpper(key)] = object[key]
							delete(object, key)
						case "null":
							object[key] = nil
						}
					})
					if mode == "duplicate" {
						quoted, _ := json.Marshal(key)
						// Add the duplicate inside this exact nested object, not an earlier matching key.
						candidate = replaceRawObject(t, tc.payload, tc.path, func(raw []byte) []byte {
							prefix := append(append([]byte{'{'}, quoted...), []byte(":null,")...)
							return append(prefix, raw[1:]...)
						})
					}
					assertRejected(t, candidate, build, request, tc.response)
				}
			}
			candidate := changeObject(t, tc.payload, tc.path, func(object map[string]any) { object["unexpected"] = "PRIVATE_CANARY" })
			assertRejected(t, candidate, build, request, tc.response)
		})
	}
	// Escaped duplicate keys must compare by decoded name, not input spelling.
	duplicate := bytes.Replace(req, []byte(`"source":`), []byte(`"\u0073ource":"system","source":`), 1)
	assertRejected(t, duplicate, build, request, false)
}

func TestInvalidFramingAndByteBounds(t *testing.T) {
	build, request, _, req, resp := packets(t)
	for _, tc := range []struct {
		payload  []byte
		response bool
		maximum  int
	}{{req, false, MaxRequestBytes}, {resp, true, MaxResponseBytes}} {
		for _, invalid := range [][]byte{
			nil, []byte("null"), []byte("[]"), []byte("{}"), append([]byte{0xef, 0xbb, 0xbf}, tc.payload...),
			append(append([]byte{}, tc.payload...), []byte("{}")...), append(append([]byte{}, tc.payload...), 0xff), tc.payload[:len(tc.payload)-1],
			[]byte(`{"unexpected":` + strings.Repeat("[", 9) + `0` + strings.Repeat("]", 9) + `}`),
		} {
			assertRejected(t, invalid, build, request, tc.response)
		}
		atLimit := append(append([]byte{}, tc.payload...), bytes.Repeat([]byte(" "), tc.maximum-len(tc.payload))...)
		if tc.response {
			if _, err := DecodeResponse(atLimit, build, request); err != nil {
				t.Fatal("response at byte limit rejected")
			}
		} else {
			if _, err := DecodeRequest(atLimit, build); err != nil {
				t.Fatal("request at byte limit rejected")
			}
		}
		assertRejected(t, append(atLimit, ' '), build, request, tc.response)
	}
}

func TestBuildAndCorrelationMustMatch(t *testing.T) {
	build, request, batch, req, resp := packets(t)
	for key, value := range map[string]string{"version": "0.1.0-preview.4", "commit": strings.Repeat("b", 40), "os": "windows", "arch": "arm64"} {
		for _, packet := range []struct {
			payload  []byte
			response bool
		}{{req, false}, {resp, true}} {
			candidate := changeObject(t, packet.payload, []any{"build"}, func(object map[string]any) { object[key] = value })
			assertRejected(t, candidate, build, request, packet.response)
		}
	}
	for _, packet := range []struct {
		payload  []byte
		response bool
	}{{req, false}, {resp, true}} {
		candidate := changeObject(t, packet.payload, nil, func(object map[string]any) { object["protocol"] = "observer-log-helper/v2" })
		assertRejected(t, candidate, build, request, packet.response)
	}
	for key, value := range map[string]any{"source": "application", "expected_revision": 8, "query_started_at": formatTime(fixtureTime.Add(time.Nanosecond))} {
		candidate := changeObject(t, resp, []any{"batch"}, func(object map[string]any) { object[key] = value })
		assertRejected(t, candidate, build, request, true)
	}
	for _, invalid := range []Build{
		{Version: "dev", Commit: build.Commit, OS: build.OS, Arch: build.Arch},
		{Version: "0.1.0", Commit: build.Commit, OS: build.OS, Arch: build.Arch},
		{Version: build.Version, Commit: "short", OS: build.OS, Arch: build.Arch},
		{Version: build.Version, Commit: build.Commit, OS: "darwin", Arch: build.Arch},
		{Version: build.Version, Commit: build.Commit, OS: build.OS, Arch: "386"},
	} {
		if _, err := EncodeRequest(invalid, request); err == nil {
			t.Fatal("invalid trusted build accepted")
		}
		if _, err := EncodeResponse(invalid, request, batch); err == nil {
			t.Fatal("invalid response build accepted")
		}
	}
	request.Source = logobs.SourceApplication
	if _, err := EncodeRequest(build, request); err == nil {
		t.Fatal("Linux application source accepted")
	}
}

func TestCanonicalTimesAndIntegerForms(t *testing.T) {
	build, request, _, req, resp := packets(t)
	for _, value := range []string{"2026-09-10T12:00:00.000000123+00:00", "2026-09-10T12:00:00.0000001230Z", "2026-09-10T12:00:00,000000123Z", "10000-01-01T00:00:00Z"} {
		candidate := changeObject(t, req, nil, func(object map[string]any) { object["query_started_at"] = value })
		assertRejected(t, candidate, build, request, false)
		candidate = changeObject(t, resp, []any{"batch"}, func(object map[string]any) { object["started_at"] = value })
		assertRejected(t, candidate, build, request, true)
	}
	for _, value := range []string{"7.0", "7e0", "-1", "18446744073709551616", "\"7\""} {
		candidate := bytes.Replace(req, []byte(`"revision":7`), []byte(`"revision":`+value), 1)
		assertRejected(t, candidate, build, request, false)
	}
}

func TestPrivateCursorBoundsAndCanonicalEncoding(t *testing.T) {
	build, request, batch, req, resp := packets(t)
	for _, value := range []string{"", "Zg", "Zh==", "Zg==\n", "_w==", base64.StdEncoding.EncodeToString(make([]byte, logobs.MaxCheckpointBytes+1))} {
		candidate := changeObject(t, req, []any{"checkpoint"}, func(object map[string]any) { object["opaque_base64"] = value })
		assertRejected(t, candidate, build, request, false)
		candidate = changeObject(t, resp, []any{"batch"}, func(object map[string]any) { object["next_opaque_base64"] = value })
		assertRejected(t, candidate, build, request, true)
	}
	request.Checkpoint.Opaque = bytes.Repeat([]byte{0xff}, logobs.MaxCheckpointBytes)
	batch.NextOpaque = bytes.Repeat([]byte{0xff}, logobs.MaxCheckpointBytes)
	payload, err := EncodeRequest(build, request)
	if err != nil {
		t.Fatal("maximum cursor encode rejected")
	}
	if _, err := DecodeRequest(payload, build); err != nil {
		t.Fatal("maximum request cursor decode rejected")
	}
	payload, err = EncodeResponse(build, request, batch)
	if err != nil {
		t.Fatal("maximum response cursor encode rejected")
	}
	if _, err := DecodeResponse(payload, build, request); err != nil {
		t.Fatal("maximum response cursor decode rejected")
	}
	request.Checkpoint.Opaque = append(request.Checkpoint.Opaque, 1)
	if _, err := EncodeRequest(build, request); err == nil {
		t.Fatal("oversized request cursor encoded")
	}
	request.Checkpoint.Opaque = request.Checkpoint.Opaque[:logobs.MaxCheckpointBytes]
	batch.NextOpaque = append(batch.NextOpaque, 1)
	if _, err := EncodeResponse(build, request, batch); err == nil {
		t.Fatal("oversized response cursor encoded")
	}
}

func TestResetCorrelationAndNormalizedStates(t *testing.T) {
	build, request, _ := fixture()
	reason := logobs.ReasonCheckpointReset
	reset := logobs.Batch{Kind: logobs.BatchResetEstablished, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
		QueryStartedAt: fixtureTime, StartedAt: fixtureTime, FinishedAt: fixtureTime, SupportState: logobs.SupportSupported,
		CollectionState: logobs.CollectionPartial, ReasonCode: &reason, ExaminedCount: 2, ProbeCount: 2, NextOpaque: []byte("synthetic-tail")}
	for _, pending := range []bool{false, true} {
		candidate := request.Clone()
		candidate.Checkpoint.ResetPending = pending
		if pending {
			candidate.Checkpoint.Opaque = nil
			reset.ProbeCount = 1
			reset.ExaminedCount = 1
		}
		payload, err := EncodeResponse(build, candidate, reset)
		if err != nil {
			t.Fatal("valid reset encode rejected")
		}
		if _, err := DecodeResponse(payload, build, candidate); err != nil {
			t.Fatal("valid reset decode rejected")
		}
		reset.Kind = logobs.BatchResetPending
		reset.NextOpaque = nil
		payload, err = EncodeResponse(build, candidate, reset)
		if err != nil {
			t.Fatal("valid pending reset encode rejected")
		}
		if _, err := DecodeResponse(payload, build, candidate); err != nil {
			t.Fatal("valid pending reset decode rejected")
		}
		reset.Kind = logobs.BatchResetEstablished
		reset.NextOpaque = []byte("synthetic-tail")
	}
	initial := request.Clone()
	initial.Checkpoint = logobs.Checkpoint{}
	reset.ExpectedRevision = 0
	if _, err := EncodeResponse(build, initial, reset); err == nil {
		t.Fatal("initial request claimed stale reset")
	}
	_, _, normal := fixture()
	request.Checkpoint.ResetPending = true
	request.Checkpoint.Opaque = nil
	if _, err := EncodeResponse(build, request, normal); err == nil {
		t.Fatal("pending request resumed normal in same attempt")
	}
}

func TestDomainJSONStillOmitsPrivateValues(t *testing.T) {
	_, request, batch, _, _ := packets(t)
	for _, value := range []any{request, request.Checkpoint, batch} {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal("domain JSON failed")
		}
		for _, private := range [][]byte{request.Checkpoint.Opaque, batch.NextOpaque} {
			if bytes.Contains(payload, private) || bytes.Contains(payload, []byte(base64.StdEncoding.EncodeToString(private))) {
				t.Fatal("private cursor leaked to domain JSON")
			}
		}
	}
}

func TestMaximumRowsAndEmptyBatches(t *testing.T) {
	build, request, _ := fixture()
	request.Checkpoint = logobs.Checkpoint{}
	batch := logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source,
		QueryStartedAt: fixtureTime, StartedAt: fixtureTime, FinishedAt: fixtureTime,
		SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionOK, CaughtUp: true}
	for _, count := range []int{0, logobs.MaxAcceptedEvents} {
		batch.Events = make([]logobs.Event, count)
		batch.ExaminedCount = uint32(count)
		batch.ProbeCount = 0
		batch.NextOpaque = nil
		if count == 0 {
			batch.ProbeCount = 1
			batch.ExaminedCount = 1
		}
		batch.NextOpaque = []byte("synthetic-last-cursor")
		for i := range batch.Events {
			batch.Events[i] = logobs.Event{ObservedAt: fixtureTime, Source: request.Source, Severity: logobs.SeverityInfo, EventCode: "SYSTEMD_PRIORITY_6"}
		}
		payload, err := EncodeResponse(build, request, batch)
		if err != nil {
			t.Fatal("bounded row batch encode failed")
		}
		if _, err := DecodeResponse(payload, build, request); err != nil {
			t.Fatal("bounded row batch decode failed")
		}
		if count == logobs.MaxAcceptedEvents {
			tooMany := changeObject(t, payload, []any{"batch"}, func(object map[string]any) {
				events := object["events"].([]any)
				object["events"] = append(events, events[0])
			})
			assertRejected(t, tooMany, build, request, true)
			batch.Events = append(batch.Events, batch.Events[0])
			if _, err := EncodeResponse(build, request, batch); err == nil {
				t.Fatal("oversized event array encoded")
			}
		}
	}
	if _, err := encodeBounded(strings.Repeat("x", MaxRequestBytes), MaxRequestBytes); err != ErrInvalid {
		t.Fatal("encoder omitted framing from byte bound")
	}
}

func assertRejected(t *testing.T, payload []byte, build Build, request logobs.ReadRequest, response bool) {
	t.Helper()
	var err error
	if response {
		var result logobs.Batch
		result, err = DecodeResponse(payload, build, request)
		if !reflect.DeepEqual(result, logobs.Batch{}) {
			t.Fatal("invalid response returned partial state")
		}
	} else {
		var result logobs.ReadRequest
		result, err = DecodeRequest(payload, build)
		if !reflect.DeepEqual(result, logobs.ReadRequest{}) {
			t.Fatal("invalid request returned partial state")
		}
	}
	if err != ErrInvalid && err != ErrIdentityMismatch {
		t.Fatal("invalid packet did not return a fixed error")
	}
}

func objectAt(t *testing.T, payload []byte, path []any) map[string]any {
	t.Helper()
	var value any
	if json.Unmarshal(payload, &value) != nil {
		t.Fatal("invalid test fixture")
	}
	for _, part := range path {
		switch key := part.(type) {
		case string:
			value = value.(map[string]any)[key]
		case int:
			value = value.([]any)[key]
		}
	}
	return value.(map[string]any)
}
func changeObject(t *testing.T, payload []byte, path []any, change func(map[string]any)) []byte {
	t.Helper()
	var value any
	if json.Unmarshal(payload, &value) != nil {
		t.Fatal("invalid test fixture")
	}
	object := value
	for _, part := range path {
		switch key := part.(type) {
		case string:
			object = object.(map[string]any)[key]
		case int:
			object = object.([]any)[key]
		}
	}
	change(object.(map[string]any))
	changed, err := json.Marshal(value)
	if err != nil {
		t.Fatal("invalid test mutation")
	}
	return changed
}

func replaceRawObject(t *testing.T, payload []byte, path []any, change func([]byte) []byte) []byte {
	t.Helper()
	if len(path) == 0 {
		return change(payload)
	}
	var value any
	switch key := path[0].(type) {
	case string:
		var object map[string]json.RawMessage
		if json.Unmarshal(payload, &object) != nil {
			t.Fatal("invalid object fixture")
		}
		object[key] = replaceRawObject(t, object[key], path[1:], change)
		value = object
	case int:
		var array []json.RawMessage
		if json.Unmarshal(payload, &array) != nil {
			t.Fatal("invalid array fixture")
		}
		array[key] = replaceRawObject(t, array[key], path[1:], change)
		value = array
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("invalid raw fixture mutation")
	}
	return encoded
}
