package template

import (
	"testing"
)

func TestPathCreateDBDir(t *testing.T) {
	if err := PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
}
