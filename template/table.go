package template

import (
	"path/filepath"

	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/util"
)

func PathCreateTableDirs(tableNames []string) error {
	for _, tableName := range tableNames {
		p := filepath.Join(conf.Args.OutputPath, db.NormalizeTableName(conf.Args.DBName), db.NormalizeTableName(tableName))
		if err := util.EnsureDir(p); err != nil {
			return nabu.FromError(err).WithArgs(p).Log()
		}
	}
	return nil
}
