package containerobs

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var sensitiveMetadataPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@[^\s]+`),
	regexp.MustCompile(`(?i)\b[^\s/@:]+:[^\s/@]+@[a-z0-9.-]+(?::[0-9]+)?(?:/[^\s]*)?`),
	regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`),
	regexp.MustCompile(`(?i)\b(?:token|password|passwd|secret|api[_-]?key|access[_-]?key)[=:][^\s]+`),
	regexp.MustCompile(`(?i)\bbearer\s+[^\s]+`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
}

func sanitizeMetadata(value string, maxRunes int, fallback string) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	for _, pattern := range sensitiveMetadataPatterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	value = truncateRunes(value, maxRunes)
	if value == "" {
		return fallback
	}
	return value
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
