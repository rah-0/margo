package template

import (
	"path/filepath"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/util"
)

func PathCreateDBDir() error {
	return util.EnsureDir(filepath.Join(conf.Args.OutputPath, db.NormalizeTableName(conf.Args.DBName)))
}
