package template

import (
	"testing"

	"github.com/rah-0/margo/db"
)

func TestCreateGoFile(t *testing.T) {
	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFields(conn, tn)
		if err != nil {
			t.Fatal(err)
		}
		if err := CreateGoFile(tn, tfs); err != nil {
			t.Fatal(err)
		}
	}
}
