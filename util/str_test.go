package util

import "testing"

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{"", ""},
		{"a", "A"},
		{"A", "A"},
		{"abc", "Abc"},
		{"Abc", "Abc"},
		{"ABC", "Abc"},
		{"äbc", "Äbc"},
		{"ÄBC", "Äbc"},
		{"1test", "1test"},
		{"тест", "Тест"}, // Cyrillic
	}

	for _, tt := range tests {
		got := Capitalize(tt.input)
		if got != tt.output {
			t.Errorf("Capitalize(%q) = %q; want %q", tt.input, got, tt.output)
		}
	}
}
