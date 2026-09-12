package errs

import "errors"

var (
	ErrSelectStarNotAllowed = errors.New("query: SELECT * is not allowed")
)
