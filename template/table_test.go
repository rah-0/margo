package template

import (
	"testing"

	"github.com/rah-0/margo/db"
)

func TestPathCreateTableDirs(t *testing.T) {
	tableNames, err := db.GetDbTables(conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := PathCreateTableDirs(tableNames); err != nil {
		t.Fatal(err)
	}
}
