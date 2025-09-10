package util

import (
	"strings"
	"unicode"
)

// Capitalize returns the string with only the first rune uppercased, rest lowercased.
func Capitalize(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	for i := 1; i < len(r); i++ {
		r[i] = unicode.ToLower(r[i])
	}
	return string(r)
}

// TrimPrefixCase trims the given prefix (case-sensitive) if present,
// returning the remainder and true. If not present, it returns the original string and false.
func TrimPrefixCase(s, prefix string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(s, prefix)), true
	}
	return s, false
}

// ParseResultMode normalizes a -- ResultMode: tag into "one", "many" or "exec".
// Defaults to "many". If the tag has multiple values (e.g. "one, many"), only the first is used.
func ParseResultMode(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "one", "exec", "many":
		return v
	default:
		return "many"
	}
}
