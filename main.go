package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/rah-0/slogx"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/template"
)

func main() {
	slogx.SetDefault(slogx.Options{
		Format: slogx.Text,
		Writer: os.Stderr,
	})

	if err := run(); err != nil {
		slogx.Error("margo generation failed", err)
		os.Exit(1)
	}

	slog.Info("margo generation completed")
}

func run() (err error) {
	if err := conf.CheckFlags(); err != nil {
		return fmt.Errorf("check flags: %w", err)
	}

	conn, err := db.Connect()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()

	if err = template.PathCreateOutputDir(); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err = template.PathCreateDBDir(); err != nil {
		return fmt.Errorf("create database output directory: %w", err)
	}

	tableNames, err := db.GetDbTables(conn)
	if err != nil {
		return fmt.Errorf("read database tables: %w", err)
	}

	if err = template.PathCreateTableDirs(tableNames); err != nil {
		return fmt.Errorf("create table output directories: %w", err)
	}

	nqs, err := template.CreateGoFileQueries(tableNames)
	if err != nil {
		return fmt.Errorf("create queries file: %w", err)
	}

	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFields(conn, tn)
		if err != nil {
			return fmt.Errorf("read fields for table %q: %w", tn, err)
		}

		var tnqs []conf.NamedQuery
		for _, nq := range nqs {
			if nq.MapAs == tn {
				tnqs = append(tnqs, nq)
			}
		}

		if err := template.CreateGoFileEntity(tn, tfs, tnqs); err != nil {
			return fmt.Errorf("create entity file for table %q: %w", tn, err)
		}
	}

	return nil
}
