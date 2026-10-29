package workloadprojection

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A second egress boundary is deliberate: local process names do not have the
// Docker adapter's redaction. These patterns are defense in depth, not anonymity.
var sensitive = regexp.MustCompile(`(?i)(hl[ec]_[A-Za-z0-9_-]{43}|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+|[a-z][a-z0-9+.-]*://|\b[^\s/@:]+:[^\s/@]+@|\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}|\b(token|password|passwd|secret|api[_-]?key|access[_-]?key)[=:]|\bbearer\s+|\bgh[pousr]_[A-Za-z0-9]{20,}|\bgithub` + `_pat_[A-Za-z0-9_]{20,}|\bAKIA[0-9A-Z]{16}\b|\b(10\.\d{1,3}|192\.168|172\.(1[6-9]|2\d|3[01]))\.\d{1,3}\.\d{1,3}\b|\[[0-9a-f:]+:[0-9a-f:]+\])`)

func display(raw string, maxBytes int, basename bool) (string, error) {
	if len(raw) > 1024 || !utf8.ValidString(raw) {
		return "", ErrData
	}
	value := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw)
	value = strings.Join(strings.Fields(value), " ")
	if sensitive.MatchString(value) {
		return "[REDACTED]", nil
	}
	if basename {
		value = strings.ReplaceAll(value, "\\", "/")
		if i := strings.LastIndexByte(value, '/'); i >= 0 {
			value = value[i+1:]
		}
	}
	for len(value) > maxBytes {
		_, n := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-n]
	}
	if value == "" {
		value = "unknown"
	}
	return value, nil
}

func processState(raw string) string {
	switch raw {
	case "running", "R":
		return "running"
	case "sleep", "sleeping", "S":
		return "sleeping"
	case "stop", "stopped", "tracing-stop", "T":
		return "stopped"
	case "zombie", "Z":
		return "zombie"
	case "idle", "I":
		return "idle"
	case "wait", "waiting", "disk-sleep", "D":
		return "waiting"
	default:
		return "unknown"
	}
}
