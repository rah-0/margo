package template

import (
	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/util"
)

func PathCreateOutputDir() error {
	return util.EnsureDir(conf.Args.OutputPath)
}
