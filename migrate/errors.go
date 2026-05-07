package migrate

import "errors"

var (
	ErrPathRequired     = errors.New("migrate: opts.Path is required")
	ErrInvalidFilename  = errors.New("migrate: filename does not match {version}_{name}.{up|down}.sql")
	ErrDuplicateVersion = errors.New("migrate: duplicate migration version")
	ErrOrphanDown       = errors.New("migrate: down file without matching up file")
	ErrNameMismatch     = errors.New("migrate: up/down filename names differ for the same version")
	ErrLockTimeout      = errors.New("migrate: could not acquire advisory lock within timeout")
	ErrLockFailed       = errors.New("migrate: GET_LOCK returned NULL")
	ErrMissingFile      = errors.New("migrate: previously applied migration file missing on disk")
	ErrHashMismatch     = errors.New("migrate: applied migration hash does not match disk")
	ErrDirtyState       = errors.New("migrate: previous migration left the database in a dirty state")
	ErrMigrationFailed  = errors.New("migrate: migration execution failed")
	ErrNoDownFile       = errors.New("migrate: rollback requested but down file is missing")
	ErrTargetNotFound   = errors.New("migrate: target version not found on filesystem")
	ErrNonLinearHistory = errors.New("migrate: filesystem migration sits below the current applied version but is not applied")
)
