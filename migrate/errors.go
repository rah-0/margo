package migrate

import "errors"

var (
	ErrDatabaseRequired        = errors.New("migrate: opts.DB is required")
	ErrDatabaseNotSelected     = errors.New("migrate: a database must be selected")
	ErrMultiStatementsRequired = errors.New("migrate: database connection must support multiple statements")
	ErrInvalidSessionState     = errors.New("migrate: invalid database session state")
	ErrPathRequired            = errors.New("migrate: opts.Path is required")
	ErrInvalidFilename         = errors.New("migrate: filename must match {positive_version}_{name}.sql with a uint64 version")
	ErrDuplicateVersion        = errors.New("migrate: duplicate migration version")
	ErrVersionGap              = errors.New("migrate: missing migration version")
	ErrLockTimeout             = errors.New("migrate: could not acquire advisory lock within timeout")
	ErrLockFailed              = errors.New("migrate: could not acquire advisory lock")
	ErrMigrationFailed         = errors.New("migrate: migration execution failed")
)
