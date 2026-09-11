package conf

import "errors"

var (
	ErrMissingArgs           = errors.New("missing required arguments")
	ErrOutputPathInvalid     = errors.New("outputPath is not accessible")
	ErrOutputPathNotDir      = errors.New("outputPath must be a directory, not a file")
	ErrQueriesPathInvalid    = errors.New("queriesPath does not exist or is not accessible")
	ErrQueriesPathNotDir     = errors.New("queriesPath must be a directory, not a file")
	ErrMigrationsPathInvalid = errors.New("migrationsPath does not exist or is not accessible")
	ErrMigrationsPathNotDir  = errors.New("migrationsPath must be a directory, not a file")
)
