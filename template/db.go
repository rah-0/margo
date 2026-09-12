package template

import (
	"path/filepath"

	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/util"
)

func (r Renderer) PathCreateDBDir() error {
	return util.EnsureDir(filepath.Join(r.OutputPath, db.NormalizeString(r.DBName)))
}
