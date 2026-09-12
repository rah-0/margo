package errs

import "errors"

var (
	ErrDatabaseNotSelected = errors.New("database: must be selected")
)
