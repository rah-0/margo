// Package errs defines MarGO's sentinel errors for use with errors.Is.
package errs

import "errors"

var (
	ErrConnectionRequired = errors.New("margo: DB or Connection is required")
	ErrConnectionConflict = errors.New("margo: DB and Connection cannot both be supplied")
	ErrInvalidConnection  = errors.New("margo: invalid connection options")
)
