//go:build integration || benchmark

// Package integration provides the disposable infrastructure and fixtures
// used by MarGO's integration tests and benchmarks.
package integration

import (
	"context"
	"database/sql"
	_ "embed"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
)

const (
	mariaDBImage    = "mariadb:12.3.3"
	mariaDBDatabase = "margo_test"
	mariaDBUsername = "margo"
	mariaDBPassword = "margo"
	startupTimeout  = 3 * time.Minute
)

//go:embed testdata/schema.sql
var schemaSQL []byte

// Settings contains the connection values needed by generator tests.
type Settings struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
}

// MariaDB is a disposable MariaDB instance owned by a test.
type MariaDB struct {
	DB       *sql.DB
	DSN      string
	Settings Settings
}

// StartMariaDB starts MariaDB, initializes its schema, verifies connectivity,
// and registers database and container cleanup with t.
func StartMariaDB(t testing.TB) *MariaDB {
	t.Helper()

	schemaPath := materializeSchema(t)
	ctx, cancel := context.WithTimeout(t.Context(), startupTimeout)
	defer cancel()

	container, err := mariadb.Run(
		ctx,
		mariaDBImage,
		mariadb.WithDatabase(mariaDBDatabase),
		mariadb.WithUsername(mariaDBUsername),
		mariadb.WithPassword(mariaDBPassword),
		mariadb.WithScripts(schemaPath),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start MariaDB container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("resolve MariaDB host: %v", err)
	}

	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("resolve MariaDB port: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "tls=false")
	if err != nil {
		t.Fatalf("build MariaDB connection string: %v", err)
	}

	database, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB connection: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close MariaDB connection: %v", err)
		}
	})

	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}

	return &MariaDB{
		DB:  database,
		DSN: dsn,
		Settings: Settings{
			Host:     host,
			Port:     port.Port(),
			Database: mariaDBDatabase,
			Username: mariaDBUsername,
			Password: mariaDBPassword,
		},
	}
}

func materializeSchema(t testing.TB) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "schema.sql")
	if err := os.WriteFile(path, schemaSQL, 0o600); err != nil {
		t.Fatalf("materialize MariaDB schema: %v", err)
	}

	return path
}
