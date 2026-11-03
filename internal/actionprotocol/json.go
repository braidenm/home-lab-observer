// Package actionprotocol contains the closed paired action codecs. Successful
// decoding establishes structure only, never remote authority or local policy.
package actionprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var ErrInvalid = errors.New("action_protocol_invalid")

const MaxEnvelopeBytes = 16384

func strictJSON(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxEnvelopeBytes || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		token, err := d.Token()
		if err != nil {
			return ErrInvalid
		}
		switch token := token.(type) {
		case json.Delim:
			if depth >= 8 || (token != '{' && token != '[') {
				return ErrInvalid
			}
			seen := make(map[string]bool)
			for d.More() {
				if token == '{' {
					key, err := d.Token()
					name, ok := key.(string)
					if err != nil || !ok || seen[name] {
						return ErrInvalid
					}
					seen[name] = true
				}
				if value(depth+1) != nil {
					return ErrInvalid
				}
			}
			end, err := d.Token()
			if err != nil || (token == '{' && end != json.Delim('}')) || (token == '[' && end != json.Delim(']')) {
				return ErrInvalid
			}
		case json.Number:
			// Every numeric member in this protocol is a small nonnegative integer.
			text := string(token)
			if len(text) == 0 || (len(text) > 1 && text[0] == '0') {
				return ErrInvalid
			}
			for _, c := range text {
				if c < '0' || c > '9' {
					return ErrInvalid
				}
			}
		}
		return nil
	}
	if value(0) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// objectFields is used only with fixed source-owned lists by named codecs. It
// prevents encoding/json's otherwise case-insensitive struct-field matching.
func objectFields(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(object) != len(names) {
		return nil, ErrInvalid
	}
	for _, name := range names {
		if _, exists := object[name]; !exists {
			return nil, ErrInvalid
		}
	}
	return object, nil
}
