package errs

import "errors"

var (
	ErrQueriesWithoutOutput  = errors.New("path: queries require an output path")
	ErrOutputPathInvalid     = errors.New("path: output is not accessible")
	ErrOutputPathNotDir      = errors.New("path: output must be a directory")
	ErrQueriesPathInvalid    = errors.New("path: queries directory does not exist or is not accessible")
	ErrQueriesPathNotDir     = errors.New("path: queries must be a directory")
	ErrMigrationsPathInvalid = errors.New("path: migrations directory does not exist or is not accessible")
	ErrMigrationsPathNotDir  = errors.New("path: migrations must be a directory")
)
