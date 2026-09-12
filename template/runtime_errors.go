package template

import (
	"path/filepath"

	"github.com/rah-0/margo/util"
)

func (r Renderer) createGoFileErrors() error {
	directory := filepath.Join(r.OutputPath, "errs")
	if err := util.EnsureDir(directory); err != nil {
		return err
	}
	content := "// Package errs defines errors shared by generated database bindings.\npackage errs\n\n"
	content += GetCommentWarning()
	content += `import "errors"

var (
	// ErrDatabaseNotInitialized indicates that SetDB has not supplied a pool.
	ErrDatabaseNotInitialized = errors.New("database: not initialized")
	// ErrMissingReturns identifies a named query without required return metadata.
	ErrMissingReturns = errors.New("query metadata: -- Returns is required")
	// ErrMultipleRows indicates that a single-row query returned more than one row.
	ErrMultipleRows = errors.New("query one: expected one row, got multiple")
	// ErrUpdateParamsRequired identifies missing update or where fields.
	ErrUpdateParamsRequired = errors.New("update: params.Update and params.Where are required")
	// ErrExistsParamsRequired identifies a missing existence-query parameter object.
	ErrExistsParamsRequired = errors.New("exists: params are required")
)
`
	return util.WriteGoFile(filepath.Join(directory, "errors.go"), content)
}
