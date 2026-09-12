package errs

import "errors"

var (
	ErrDatabaseRequired        = errors.New("migrate: opts.DB is required")
	ErrMultiStatementsRequired = errors.New("migrate: database connection must support multiple statements")
	ErrInvalidSessionState     = errors.New("migrate: invalid database session state")
	ErrInvalidFilename         = errors.New("migrate: filename must match {positive_version}_{name}.sql with a uint64 version")
	ErrDuplicateVersion        = errors.New("migrate: duplicate migration version")
	ErrVersionGap              = errors.New("migrate: missing migration version")
	ErrLockTimeout             = errors.New("migrate: could not acquire advisory lock within timeout")
	ErrLockFailed              = errors.New("migrate: could not acquire advisory lock")
	ErrLockReleaseFailed       = errors.New("migrate: advisory lock release not confirmed")
	ErrVersionUpdateFailed     = errors.New("migrate: version update expected to affect one row")
	ErrMigrationFailed         = errors.New("migrate: migration execution failed")
)
