package logprotocol

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

// A bounded token pass rejects duplicates before ordinary struct decoding can
// collapse them, including escaped spellings of the same decoded key.
func validPacket(payload []byte, maximum int) bool {
	if len(payload) == 0 || len(payload) > maximum || !utf8.Valid(payload) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	budget := 16384
	if !scanValue(decoder, 1, &budget) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func scanValue(decoder *json.Decoder, depth int, budget *int) bool {
	if depth > 8 || *budget <= 0 {
		return false
	}
	*budget--
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return true
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			if *budget <= 0 {
				return false
			}
			*budget--
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			seen[name] = true
			if !scanValue(decoder, depth+1, budget) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !scanValue(decoder, depth+1, budget) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func objectKeys(payload []byte, required []string, nullable ...string) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil || len(object) != len(required) {
		return nil, false
	}
	for _, key := range required {
		value, present := object[key]
		if !present {
			return nil, false
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			allowed := false
			for _, name := range nullable {
				allowed = allowed || name == key
			}
			if !allowed {
				return nil, false
			}
		}
	}
	return object, true
}

func buildShape(payload []byte) bool {
	_, ok := objectKeys(payload, []string{"version", "commit", "os", "arch"})
	return ok
}

func requestShape(payload []byte) bool {
	object, ok := objectKeys(payload, []string{"protocol", "build", "source", "query_started_at", "checkpoint"})
	if !ok || !buildShape(object["build"]) {
		return false
	}
	_, ok = objectKeys(object["checkpoint"], []string{"revision", "reset_pending", "opaque_base64", "previous_attempt_at", "coverage_through"},
		"opaque_base64", "previous_attempt_at", "coverage_through")
	return ok
}

func responseShape(payload []byte) bool {
	object, ok := objectKeys(payload, []string{"protocol", "build", "batch"})
	if !ok || !buildShape(object["build"]) {
		return false
	}
	batch, ok := objectKeys(object["batch"], []string{
		"kind", "source", "expected_revision", "query_started_at", "started_at", "finished_at",
		"support_state", "collection_state", "reason_code", "events", "discards", "examined_count", "probe_count",
		"discarded_count", "deferred", "caught_up", "next_opaque_base64",
	}, "reason_code", "next_opaque_base64")
	if !ok {
		return false
	}
	return objectArray(batch["events"], []string{"observed_at", "source", "severity", "event_code"}) &&
		objectArray(batch["discards"], []string{"at", "count"})
}

func objectArray(payload []byte, required []string) bool {
	var values []json.RawMessage
	if json.Unmarshal(payload, &values) != nil || values == nil || len(values) > logobs.MaxAcceptedEvents {
		return false
	}
	for _, value := range values {
		if _, ok := objectKeys(value, required); !ok {
			return false
		}
	}
	return true
}
