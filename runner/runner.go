// Package runner applies MariaDB migrations and generates Go database bindings.
package runner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/structs"
	"github.com/rah-0/margo/template"
)

// Run validates the requested paths and filesystem, acquires a connection,
// applies migrations, and then generates bindings. With no paths or filesystem
// it succeeds without any work.
// It never closes a caller-supplied DB or changes process configuration.
func Run(ctx context.Context, opts Options) (err error) {
	migrationsEnabled := opts.MigrationsPath != "" || opts.MigrationsFS != nil
	if opts.OutputPath == "" && opts.QueriesPath == "" && !migrationsEnabled {
		return nil
	}
	if err := opts.validate(ctx); err != nil {
		return fmt.Errorf("validate options: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	conn := opts.DB
	var database string
	if opts.Connection != nil {
		// Defaults apply to a copy; callers may reuse their connection settings.
		settings := *opts.Connection
		if settings.Port == "" {
			settings.Port = "3306"
		}
		database = settings.Database
		conn, err = db.ConnectContext(ctx, settings, migrationsEnabled)
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
			}
		}()
	} else {
		var selected sql.NullString
		if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&selected); err != nil {
			return fmt.Errorf("read selected database: %w", err)
		}
		if !selected.Valid || selected.String == "" {
			return errs.ErrDatabaseNotSelected
		}
		database = selected.String
	}

	if migrationsEnabled {
		if err := migrate.Run(ctx, migrate.Options{DB: conn, Path: opts.MigrationsPath, FS: opts.MigrationsFS}); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.OutputPath == "" {
		return nil
	}

	renderer := template.Renderer{DBName: database, OutputPath: opts.OutputPath, QueriesPath: opts.QueriesPath}
	if err = renderer.PathCreateOutputDir(); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	// Resolve symlinks before joining child paths, which otherwise cleans away
	// parent components such as link/.. and can change the output destination.
	outputPath, err := filepath.EvalSymlinks(opts.OutputPath)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	renderer.OutputPath = outputPath

	if err = renderer.PathCreateDBDir(); err != nil {
		return fmt.Errorf("create database output directory: %w", err)
	}

	tableNames, err := db.GetDbTablesContext(ctx, conn, database)
	if err != nil {
		return fmt.Errorf("read database tables: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err = renderer.PathCreateTableDirs(tableNames); err != nil {
		return fmt.Errorf("create table output directories: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	nqs, err := renderer.CreateGoFileQueries(tableNames)
	if err != nil {
		return fmt.Errorf("create queries file: %w", err)
	}

	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFieldsContext(ctx, conn, database, tn)
		if err != nil {
			return fmt.Errorf("read fields for table %q: %w", tn, err)
		}

		var tnqs []structs.NamedQuery
		for _, nq := range nqs {
			if nq.MapAs == tn {
				tnqs = append(tnqs, nq)
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		if err := renderer.CreateGoFileEntity(tn, tfs, tnqs); err != nil {
			return fmt.Errorf("create entity file for table %q: %w", tn, err)
		}
	}

	return nil
}
