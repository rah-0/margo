package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/rah-0/margo/structs"
)

// ConnectContext opens the configured database with cancellation support.
// When migrations are enabled, it creates the database if missing and enables
// whole-file SQL execution on the returned pool.
func ConnectContext(ctx context.Context, opts structs.ConnectionOptions, migrations bool) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	// Return connection errors without the driver's default stderr logging.
	// This applies only to owned pools; borrowed pools keep caller configuration.
	cfg.Logger = &mysql.NopLogger{}
	cfg.User = opts.User
	cfg.Passwd = opts.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(opts.Host, opts.Port)

	if migrations {
		server, err := openConnection(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("connect to database server: %w", err)
		}
		_, createErr := server.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+strings.ReplaceAll(opts.Database, "`", "``")+"`")
		closeErr := server.Close()
		if createErr != nil {
			createErr = fmt.Errorf("create database %q: %w", opts.Database, createErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close database server connection: %w", closeErr)
		}
		if err := errors.Join(createErr, closeErr); err != nil {
			return nil, err
		}
		cfg.MultiStatements = true
	}

	cfg.DBName = opts.Database
	return openConnection(ctx, cfg)
}

func openConnection(ctx context.Context, cfg *mysql.Config) (*sql.DB, error) {
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("configure database connection: %w", err)
	}
	conn := sql.OpenDB(connector)

	conn.SetMaxIdleConns(runtime.NumCPU())
	conn.SetConnMaxLifetime(time.Minute * 5)
	conn.SetConnMaxIdleTime(time.Minute * 1)

	if err = conn.PingContext(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return conn, nil
}
