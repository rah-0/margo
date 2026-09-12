package template

import (
	"fmt"
	"path/filepath"

	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/util"
)

func (r Renderer) PathCreateTableDirs(tableNames []string) error {
	for _, tableName := range tableNames {
		p := filepath.Join(r.OutputPath, db.NormalizeString(r.DBName), db.NormalizeString(tableName))
		if err := util.EnsureDir(p); err != nil {
			return fmt.Errorf("create output directory for table %q: %w", tableName, err)
		}
	}
	return nil
}
