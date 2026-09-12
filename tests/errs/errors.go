// Package errs contains sentinel errors used by MarGO's test helpers.
package errs

import "errors"

var (
	ErrConnectorRequired     = errors.New("test driver: use connector")
	ErrUnexpectedTransaction = errors.New("test driver: unexpected transaction")
	ErrUnexpectedPrepare     = errors.New("test driver: unexpected prepare")
	ErrUnexpectedSchemaQuery = errors.New("test driver: unexpected schema query")
	ErrSchemaUnavailable     = errors.New("schema: unavailable")
	ErrModuleRootNotFound    = errors.New("module root: not found from test directory or source")
)
