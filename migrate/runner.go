package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/rah-0/margo/errs"
)

// Run applies consecutive SQL migrations newer than the stored database version.
// It stops on the first error and leaves the caller's pool open. SQL must tolerate
// retries because execution and version updates are not one atomic operation.
// Each file must leave the original database selected, autocommit enabled, and
// no transaction open. The dedicated connection is discarded after the run.
// Only pending file bodies are read, unchanged, from FS. Cancellation
// is checked between operations; a blocking filesystem read cannot be interrupted.
func Run(ctx context.Context, opts Options) (err error) {
	if opts.DB == nil {
		return errs.ErrDatabaseRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	migrations, err := Discover(opts.FS)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := opts.DB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer func() {
		err = errors.Join(err, discardConnection(conn))
	}()

	database, err := validateSession(ctx, conn, "")
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "DO 0; DO 0"); err != nil {
		return fmt.Errorf("%w: %w", errs.ErrMultiStatementsRequired, err)
	}
	name := lockName(database)
	if err := acquireLock(ctx, conn, name); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, releaseLock(conn, name))
	}()

	current, err := initializeVersion(ctx, conn)
	if err != nil {
		return err
	}
	pending, err := PendingMigrations(migrations, current)
	if err != nil {
		return err
	}
	for _, migration := range pending {
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := fs.ReadFile(opts.FS, migration.Path)
		if err != nil {
			return fmt.Errorf("%w: read %q (version %d): %w", errs.ErrMigrationFailed, migration.Path, migration.Version, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("%w: execute %q (version %d): %w", errs.ErrMigrationFailed, migration.Path, migration.Version, err)
		}
		if _, err := validateSession(ctx, conn, database); err != nil {
			return fmt.Errorf("%w: validate session after %q (version %d): %w", errs.ErrMigrationFailed, migration.Path, migration.Version, err)
		}
		if err := saveVersion(ctx, conn, current, migration.Version); err != nil {
			return fmt.Errorf("%w: save version after %q (version %d): %w", errs.ErrMigrationFailed, migration.Path, migration.Version, err)
		}
		current = migration.Version
	}
	return nil
}
