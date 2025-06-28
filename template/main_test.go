package template

import (
	"database/sql"
	"testing"

	"github.com/rah-0/testmark/testutil"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
)

var (
	conn *sql.DB
	err  error
)

func TestMain(m *testing.M) {
	testutil.TestMainWrapper(testutil.TestConfig{
		M: m,
		LoadResources: func() error {
			conf.CheckFlags()
			conn, err = db.Connect()
			return err
		},
		UnloadResources: func() error {
			return conn.Close()
		},
	})
}
