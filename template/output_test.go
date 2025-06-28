package template

import (
	"testing"
)

func TestPathCreateOutputDir(t *testing.T) {
	if err := PathCreateOutputDir(); err != nil {
		t.Fatal(err)
	}
}
