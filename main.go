package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rah-0/slogx"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/template"
)

func main() {
	slogx.SetDefault(slogx.Options{
		Format: slogx.Text,
		Writer: os.Stderr,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		slogx.Error("margo failed", err)
		os.Exit(1)
	}

	switch {
	case conf.Args.OutputPath == "" && conf.Args.QueriesPath == "" && conf.Args.MigrationsPath == "":
		slog.WarnContext(ctx, "no action requested: specify -outputPath, -queriesPath, or -migrationsPath")
	case conf.Args.OutputPath == "":
		slog.InfoContext(ctx, "margo migrations completed")
	default:
		slog.InfoContext(ctx, "margo generation completed")
	}
}

func run(ctx context.Context) (err error) {
	if err := conf.CheckFlags(); err != nil {
		return fmt.Errorf("check flags: %w", err)
	}
	if conf.Args.OutputPath == "" && conf.Args.QueriesPath == "" && conf.Args.MigrationsPath == "" {
		return nil
	}

	conn, err := db.ConnectContext(ctx)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	if conf.Args.MigrationsPath != "" {
		if err := migrate.Run(ctx, migrate.Options{DB: conn, Path: conf.Args.MigrationsPath}); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if conf.Args.OutputPath == "" {
		return nil
	}

	if err = template.PathCreateOutputDir(); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	// Resolve symlinks before joining child paths, which otherwise cleans away
	// parent components such as link/.. and can change the output destination.
	outputPath, err := filepath.EvalSymlinks(conf.Args.OutputPath)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	conf.Args.OutputPath = outputPath

	if err = template.PathCreateDBDir(); err != nil {
		return fmt.Errorf("create database output directory: %w", err)
	}

	tableNames, err := db.GetDbTablesContext(ctx, conn)
	if err != nil {
		return fmt.Errorf("read database tables: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err = template.PathCreateTableDirs(tableNames); err != nil {
		return fmt.Errorf("create table output directories: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	nqs, err := template.CreateGoFileQueries(tableNames)
	if err != nil {
		return fmt.Errorf("create queries file: %w", err)
	}

	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFieldsContext(ctx, conn, tn)
		if err != nil {
			return fmt.Errorf("read fields for table %q: %w", tn, err)
		}

		var tnqs []conf.NamedQuery
		for _, nq := range nqs {
			if nq.MapAs == tn {
				tnqs = append(tnqs, nq)
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}
		if err := template.CreateGoFileEntity(tn, tfs, tnqs); err != nil {
			return fmt.Errorf("create entity file for table %q: %w", tn, err)
		}
	}

	return nil
}
