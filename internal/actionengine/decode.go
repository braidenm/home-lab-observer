package actionengine

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// Three fixed call sites select literal case-sensitive keys. Unknown properties
// are bounded transient input, not struct fields retained or exposed by Engine.
func selectedObject(body []byte, keys ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(body) {
		return nil, ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrUnavailable
	}
	fields := make(map[string]json.RawMessage, len(keys))
	for d.More() {
		token, err = d.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, ErrUnavailable
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, ErrUnavailable
		}
		for _, key := range keys {
			if name == key {
				if _, duplicate := fields[key]; duplicate {
					return nil, ErrUnavailable
				}
				fields[key] = value
			}
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrUnavailable
	}
	if _, err = d.Token(); err != io.EOF || len(fields) != len(keys) {
		return nil, ErrUnavailable
	}
	return fields, nil
}
func versionNumber(raw json.RawMessage) (int, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil || len(text) > 7 {
		return 0, false
	}
	parts := strings.Split(text, ".")
	if len(parts) != 2 {
		return 0, false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || major < 1 || major > 999 || minor < 0 || minor > 999 || strconv.Itoa(major)+"."+strconv.Itoa(minor) != text {
		return 0, false
	}
	return major*1000 + minor, true
}
func supportedVersion(body []byte) error {
	f, err := selectedObject(body, "ApiVersion", "MinAPIVersion", "Os")
	if err != nil {
		return err
	}
	max, okMax := versionNumber(f["ApiVersion"])
	min, okMin := versionNumber(f["MinAPIVersion"])
	var os string
	if !okMax || !okMin || min > max || json.Unmarshal(f["Os"], &os) != nil {
		return ErrUnavailable
	}
	if os != "linux" || min > 1045 || max < 1045 {
		return ErrUnsupported
	}
	return nil
}
func inspectState(body []byte, expected string) (actioncore.State, error) {
	f, err := selectedObject(body, "Id", "State")
	if err != nil {
		return actioncore.UnknownState, err
	}
	var id string
	if json.Unmarshal(f["Id"], &id) != nil || !validID(id) || id != expected {
		return actioncore.UnknownState, ErrUnavailable
	}
	state, err := selectedObject(f["State"], "Status")
	if err != nil {
		return actioncore.UnknownState, err
	}
	var status string
	if json.Unmarshal(state["Status"], &status) != nil || status == "" {
		return actioncore.UnknownState, ErrUnavailable
	}
	switch actioncore.State(status) {
	case actioncore.Created, actioncore.Exited, actioncore.Running, actioncore.Paused, actioncore.Restarting, actioncore.Removing, actioncore.Dead:
		return actioncore.State(status), nil
	default:
		return actioncore.UnknownState, nil
	}
}
