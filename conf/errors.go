package conf

import "errors"

var (
	ErrMissingArgs        = errors.New("missing required arguments")
	ErrQueriesPathInvalid = errors.New("queriesPath does not exist or is not accessible")
	ErrQueriesPathNotDir  = errors.New("queriesPath must be a directory, not a file")
)
