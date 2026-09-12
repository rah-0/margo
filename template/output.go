package template

import (
	"github.com/rah-0/margo/util"
)

func (r Renderer) PathCreateOutputDir() error {
	return util.EnsureDir(r.OutputPath)
}
