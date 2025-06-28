package template

import (
	"testing"
)

func TestPathCreateTableDirs(t *testing.T) {
	if err := PathCreateTableDirs(tableNames); err != nil {
		t.Fatal(err)
	}
}
