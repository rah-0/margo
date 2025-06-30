package template

import (
	"database/sql"
	"testing"

	"github.com/rah-0/testmark/testutil"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
)

var (
	tableNames []string
	conn       *sql.DB
)

func TestMain(m *testing.M) {
	testutil.TestMainWrapper(testutil.TestConfig{
		M: m,
		LoadResources: func() error {
			var err error
			conf.CheckFlags()

			conn, err = db.Connect()
			if err != nil {
				return err
			}

			tableNames, err = db.GetDbTables(conn)
			if err != nil {
				return err
			}

			return nil
		},
		UnloadResources: func() error {
			return conn.Close()
		},
	})
}
