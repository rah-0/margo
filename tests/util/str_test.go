package util_test

import (
	"testing"

	"github.com/rah-0/margo/util"
)

type capitalizeCase struct {
	input  string
	output string
}

func TestCapitalize(t *testing.T) {
	tests := []capitalizeCase{
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
		got := util.Capitalize(tt.input)
		if got != tt.output {
			t.Errorf("Capitalize(%q) = %q; want %q", tt.input, got, tt.output)
		}
	}
}
