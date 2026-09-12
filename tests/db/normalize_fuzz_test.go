package db_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/rah-0/margo/db"
)

func FuzzNormalizeString(f *testing.F) {
	for _, input := range []string{
		"", "a", "ABCDx", "HTTPConnection", "userID_stats", "user2fa",
		" _.- ", "foo\tbar_baz", "foo\u00a0bar", "a\x00b",
		"BöseÜberraschung", "foo٢bar", "東京name", "ǅǅ", "a\u0301b",
		"a!Ⅻb", "Aİ", "ı", "userID�ABC", "userID\xffABC", "\xc3_\xa9",
	} {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, input string) {
		got := db.NormalizeString(input)
		if !utf8.ValidString(got) {
			t.Fatalf("NormalizeString(%q) returned invalid UTF-8: %q", input, got)
		}
		if strings.ContainsAny(got, "_-. ") {
			t.Fatalf("NormalizeString(%q) retained a separator: %q", input, got)
		}

		// Normalization may change case and remove separators, but must preserve
		// all remaining runes in order. Decode before skipping separators so
		// malformed bytes separated by '_' cannot become a valid UTF-8 sequence.
		output := []rune(got)
		index := 0
		for _, original := range input {
			switch original {
			case '_', '-', '.', ' ':
				continue
			}
			if index >= len(output) {
				t.Fatalf("NormalizeString(%q) dropped a rune at output index %d: %q", input, index, got)
			}
			if output[index] != unicode.ToUpper(original) && output[index] != unicode.ToLower(original) {
				t.Fatalf("NormalizeString(%q) changed rune %U to %U at output index %d", input, original, output[index], index)
			}
			index++
		}
		if index != len(output) {
			t.Fatalf("NormalizeString(%q) added runes: %q", input, got)
		}
		if repeated := db.NormalizeString(input); repeated != got {
			t.Fatalf("NormalizeString(%q) changed between calls: %q then %q", input, got, repeated)
		}
	})
}
