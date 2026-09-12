package runner

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strings"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/structs"
)

// Options selects migrations, generation, and connection ownership for one run.
type Options struct {
	// DB is owned by the caller and remains open on every return path. Every
	// connection must consistently select the same existing database. Migrations
	// require multiStatements, autocommit, and no open transaction; see migrate.Run.
	DB *sql.DB
	// Connection selects pools owned by MarGO. Only migrations create a missing
	// database. Supply exactly one of DB or Connection for an active operation.
	Connection *structs.ConnectionOptions

	// OutputPath enables generation and must have an enclosing go.mod. Missing
	// directories are created only after migrations succeed.
	OutputPath string
	// QueriesPath contains named SQL query files and requires OutputPath.
	QueriesPath string
	// MigrationsPath contains numbered SQL migrations to apply before generation.
	// It is mutually exclusive with MigrationsFS.
	MigrationsPath string
	// MigrationsFS contains numbered SQL migrations directly in its root (".").
	// Use fs.Sub for a subdirectory. A non-nil FS enables migrations even when
	// empty. The caller keeps it valid and stable throughout Run and retains
	// ownership; MarGO only reads it.
	MigrationsFS fs.FS
}

type connectionField struct {
	name  string
	value string
}

func (opts Options) validate(ctx context.Context) error {
	if opts.MigrationsPath != "" && opts.MigrationsFS != nil {
		return errs.ErrMigrationsSourceConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := conf.ValidatePaths(opts.OutputPath, opts.QueriesPath, opts.MigrationsPath); err != nil {
		return err
	}
	if opts.MigrationsFS != nil {
		if _, err := fs.ReadDir(opts.MigrationsFS, "."); err != nil {
			return fmt.Errorf("read migrations filesystem root: %w", err)
		}
	}
	if opts.DB != nil && opts.Connection != nil {
		return errs.ErrConnectionConflict
	}
	if opts.DB == nil && opts.Connection == nil {
		return errs.ErrConnectionRequired
	}
	if opts.Connection != nil {
		var missing []string
		for _, field := range []connectionField{
			{"User", opts.Connection.User},
			{"Password", opts.Connection.Password},
			{"Host", opts.Connection.Host},
			{"Database", opts.Connection.Database},
		} {
			if field.value == "" {
				missing = append(missing, field.name)
			}
		}
		if len(missing) != 0 {
			return fmt.Errorf("%w: missing %s", errs.ErrInvalidConnection, strings.Join(missing, ", "))
		}
	}
	return nil
}
