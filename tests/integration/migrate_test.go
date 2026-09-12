//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
)

func TestMigrations(t *testing.T) {
	server := StartMariaDB(t)
	database := migrationPool(t, server.DSN, true, true)

	t.Run("progression and historical files", func(t *testing.T) {
		resetMigrationTables(t, database)
		path := t.TempDir()
		writeMigration(t, path, "0001_items.sql", `
CREATE TABLE migration_items (id INT PRIMARY KEY, name VARCHAR(64) NOT NULL);
CREATE PROCEDURE migration_insert()
BEGIN
    INSERT INTO migration_items VALUES (1, 'first; value');
    INSERT INTO migration_items VALUES (2, 'second');
END;
CALL migration_insert();
DROP PROCEDURE migration_insert;
`)
		writeMigration(t, path, "0002_items.sql", "INSERT INTO migration_items VALUES (3, 'third');")
		writeMigration(t, path, "README.md", "Ignored non-SQL file.")
		if err := os.Mkdir(filepath.Join(path, "ignored.sql"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeMigration(t, filepath.Join(path, "ignored.sql"), "malformed.sql", "INVALID SQL")

		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 2)
		assertMigrationCount(t, database, "migration_items", 3)
		var value string
		if err := database.QueryRowContext(t.Context(), "SELECT name FROM migration_items WHERE id = 1").Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value != "first; value" {
			t.Fatalf("SQL literal changed: %q", value)
		}

		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 2)
		assertMigrationCount(t, database, "migration_items", 3)

		writeMigration(t, path, "0001_items.sql", "INVALID SQL: an applied file must not run again")
		if err := os.Remove(filepath.Join(path, "0002_items.sql")); err != nil {
			t.Fatal(err)
		}
		writeMigration(t, path, "0003_items.sql", "INSERT INTO migration_items VALUES (4, 'fourth');")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 3)
		assertMigrationCount(t, database, "migration_items", 4)

		for version := 4; version <= 10; version++ {
			writeMigration(t, path, fmt.Sprintf("%04d_items.sql", version), "DO 0;")
		}
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 10)

		path = t.TempDir()
		writeMigration(t, path, "0011_items.sql", "INSERT INTO migration_items VALUES (5, 'fifth');")
		writeMigration(t, path, "0012_items.sql", "INSERT INTO migration_items VALUES (6, 'sixth');")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 12)
		assertMigrationCount(t, database, "migration_items", 6)
		runMigrations(t, database, t.TempDir())
		assertMigrationVersion(t, database, 12)
	})

	t.Run("gaps reject the complete pending batch", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			current uint64
			pending []uint64
		}{
			{name: "missing first", pending: []uint64{2}},
			{name: "missing intermediate", pending: []uint64{1, 3}},
			{name: "gap after stored version", current: 4, pending: []uint64{5, 7}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resetMigrationTables(t, database)
				path := t.TempDir()
				for version := uint64(1); version <= tc.current; version++ {
					writeMigration(t, path, fmt.Sprintf("%04d_setup.sql", version), "DO 0;")
				}
				runMigrations(t, database, path)
				for _, version := range tc.pending {
					writeMigration(t, path, fmt.Sprintf("%04d_items.sql", version), "CREATE TABLE migration_items (id INT PRIMARY KEY);")
				}
				err := migrate.Run(t.Context(), migrate.Options{DB: database, Path: path})
				if !errors.Is(err, errs.ErrVersionGap) {
					t.Fatalf("expected version-gap error, got %v", err)
				}
				assertMigrationVersion(t, database, tc.current)
				assertMigrationTableAbsent(t, database, "migration_items")
			})
		}
	})

	t.Run("partial SQL failure and retry", func(t *testing.T) {
		resetMigrationTables(t, database)
		path := t.TempDir()
		const retrySafeSQL = `
CREATE TABLE IF NOT EXISTS migration_items (id INT PRIMARY KEY);
INSERT IGNORE INTO migration_items VALUES (1);
`
		writeMigration(t, path, "0001_items.sql", retrySafeSQL+"INSERT INTO migration_missing_table VALUES (1);")
		writeMigration(t, path, "0002_next.sql", "CREATE TABLE migration_next (id INT PRIMARY KEY);")
		err := migrate.Run(t.Context(), migrate.Options{DB: database, Path: path})
		assertMigrationFailure(t, err, "0001_items.sql", 1146)
		assertMigrationVersion(t, database, 0)
		assertMigrationCount(t, database, "migration_items", 1)
		assertMigrationTableAbsent(t, database, "migration_next")

		writeMigration(t, path, "0001_items.sql", retrySafeSQL+"INSERT IGNORE INTO migration_items VALUES (2);")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 2)
		assertMigrationCount(t, database, "migration_items", 2)
		assertMigrationCount(t, database, "migration_next", 0)
	})

	t.Run("checkpoint failure and retry", func(t *testing.T) {
		resetMigrationTables(t, database)
		path := t.TempDir()
		runMigrations(t, database, path)
		migrationExec(t, database, `
CREATE TRIGGER migration_reject_checkpoint
BEFORE UPDATE ON `+migrate.TableName+`
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'test checkpoint failure'
`)
		writeMigration(t, path, "0001_items.sql", `
CREATE TABLE IF NOT EXISTS migration_items (id INT PRIMARY KEY);
INSERT IGNORE INTO migration_items VALUES (1);
`)
		writeMigration(t, path, "0002_next.sql", "CREATE TABLE migration_next (id INT PRIMARY KEY);")
		err := migrate.Run(t.Context(), migrate.Options{DB: database, Path: path})
		assertMigrationFailure(t, err, "0001_items.sql", 1644)
		assertMigrationVersion(t, database, 0)
		assertMigrationCount(t, database, "migration_items", 1)
		assertMigrationTableAbsent(t, database, "migration_next")

		migrationExec(t, database, "DROP TRIGGER migration_reject_checkpoint")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 2)
		assertMigrationCount(t, database, "migration_items", 1)
		assertMigrationCount(t, database, "migration_next", 0)
	})

	t.Run("missing checkpoint row and retry", func(t *testing.T) {
		resetMigrationTables(t, database)
		path := t.TempDir()
		writeMigration(t, path, "0001_checkpoint.sql", "DELETE FROM "+migrate.TableName+" WHERE id = 1;")
		writeMigration(t, path, "0002_next.sql", "CREATE TABLE migration_next (id INT PRIMARY KEY);")
		err := migrate.Run(t.Context(), migrate.Options{DB: database, Path: path})
		if !errors.Is(err, errs.ErrMigrationFailed) || !errors.Is(err, errs.ErrVersionUpdateFailed) || !strings.Contains(err.Error(), "0001_checkpoint.sql") {
			t.Fatalf("expected inspectable checkpoint row failure, got %v", err)
		}
		assertMigrationCount(t, database, migrate.TableName, 0)
		assertMigrationTableAbsent(t, database, "migration_next")

		writeMigration(t, path, "0001_checkpoint.sql", "DO 0;")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 2)
		assertMigrationCount(t, database, "migration_next", 0)
	})

	t.Run("lock release failure preserves primary error", func(t *testing.T) {
		for _, failSQL := range []bool{false, true} {
			resetMigrationTables(t, database)
			path := t.TempDir()
			statement := "DO RELEASE_ALL_LOCKS();"
			if failSQL {
				statement += " INSERT INTO migration_missing_table VALUES (1);"
			}
			writeMigration(t, path, "0001_release.sql", statement)
			err := migrate.Run(t.Context(), migrate.Options{DB: database, Path: path})
			if !errors.Is(err, errs.ErrLockReleaseFailed) {
				t.Fatalf("expected inspectable lock release failure, got %v", err)
			}
			if failSQL {
				assertMigrationFailure(t, err, "0001_release.sql", 1146)
				assertMigrationVersion(t, database, 0)
			} else {
				if errors.Is(err, errs.ErrMigrationFailed) {
					t.Fatalf("successful migration reported a primary failure: %v", err)
				}
				assertMigrationVersion(t, database, 1)
			}
		}
	})

	t.Run("connection requirements", func(t *testing.T) {
		for _, tc := range []struct {
			name            string
			multiStatements bool
			selected        bool
			want            error
		}{
			{name: "no selected database", multiStatements: true, want: errs.ErrDatabaseNotSelected},
			{name: "multiple statements disabled", selected: true, want: errs.ErrMultiStatementsRequired},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resetMigrationTables(t, database)
				pool := migrationPool(t, server.DSN, tc.multiStatements, tc.selected)
				path := t.TempDir()
				writeMigration(t, path, "0001_items.sql", "CREATE TABLE migration_items (id INT PRIMARY KEY);")
				err := migrate.Run(t.Context(), migrate.Options{DB: pool, Path: path})
				if !errors.Is(err, tc.want) {
					t.Fatalf("expected %v, got %v", tc.want, err)
				}
				if tc.want == errs.ErrDatabaseNotSelected && !errors.Is(err, errs.ErrInvalidSessionState) {
					t.Fatalf("missing database error does not retain session-state classification: %v", err)
				}
				assertMigrationTableAbsent(t, database, "migration_items")
				assertMigrationTableAbsent(t, database, migrate.TableName)
				if err := pool.PingContext(t.Context()); err != nil {
					t.Fatalf("caller-owned pool is no longer usable: %v", err)
				}
			})
		}
	})

	t.Run("invalid initial sessions stop before metadata", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			setup string
		}{
			{name: "autocommit disabled", setup: "SET autocommit = 0;"},
			{name: "transaction open", setup: "START TRANSACTION; INSERT INTO migration_items VALUES (1);"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resetMigrationTables(t, database)
				migrationExec(t, database, "CREATE TABLE migration_items (id INT PRIMARY KEY) ENGINE=InnoDB;")
				pool := migrationPool(t, server.DSN, true, true)
				pool.SetMaxOpenConns(1)
				pool.SetMaxIdleConns(1)
				migrationExec(t, pool, tc.setup)
				path := t.TempDir()
				writeMigration(t, path, "0001_next.sql", "CREATE TABLE migration_next (id INT PRIMARY KEY);")

				err := migrate.Run(t.Context(), migrate.Options{DB: pool, Path: path})
				if !errors.Is(err, errs.ErrInvalidSessionState) {
					t.Fatalf("expected initial session-state error, got %v", err)
				}
				assertMigrationTableAbsent(t, database, migrate.TableName)
				assertMigrationTableAbsent(t, database, "migration_next")
				assertMigrationCount(t, database, "migration_items", 0)
				assertMigrationSession(t, pool, server.Settings.Database)
				runMigrations(t, pool, path)
				assertMigrationVersion(t, database, 1)
			})
		}
	})

	t.Run("file boundaries preserve committed versions and isolate sessions", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			statement  string
			mysqlCode  uint16
			createsDDL bool
		}{
			{
				name:      "unfinished transaction",
				statement: "START TRANSACTION; INSERT INTO migration_items VALUES (2);",
			},
			{
				name:       "autocommit remains disabled after DDL",
				statement:  "SET autocommit = 0; CREATE TABLE migration_session_ddl (id INT PRIMARY KEY);",
				createsDDL: true,
			},
			{
				name:      "database changed",
				statement: "USE information_schema;",
			},
			{
				name:      "SQL failure inside transaction",
				statement: "START TRANSACTION; INSERT INTO migration_items VALUES (2); INSERT INTO migration_missing_table VALUES (1); COMMIT;",
				mysqlCode: 1146,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				resetMigrationTables(t, database)
				pool := migrationPool(t, server.DSN, true, true)
				pool.SetMaxOpenConns(1)
				pool.SetMaxIdleConns(1)
				path := t.TempDir()
				writeMigration(t, path, "0001_items.sql", `
CREATE TABLE migration_items (id INT PRIMARY KEY) ENGINE=InnoDB;
INSERT INTO migration_items VALUES (1);
`)
				writeMigration(t, path, "0002_session.sql", "SET @margo_migration_test = 1; SET SESSION foreign_key_checks = 0; "+tc.statement)
				writeMigration(t, path, "0003_next.sql", "CREATE TABLE migration_next (id INT PRIMARY KEY);")
				err := migrate.Run(t.Context(), migrate.Options{DB: pool, Path: path})
				if tc.mysqlCode != 0 {
					assertMigrationFailure(t, err, "0002_session.sql", tc.mysqlCode)
				} else if !errors.Is(err, errs.ErrInvalidSessionState) || !errors.Is(err, errs.ErrMigrationFailed) || !strings.Contains(err.Error(), "0002_session.sql") {
					t.Fatalf("expected file-specific session-state error, got %v", err)
				}
				assertMigrationVersion(t, database, 1)
				assertMigrationCount(t, database, "migration_items", 1)
				assertMigrationTableAbsent(t, database, "migration_next")
				if tc.createsDDL {
					assertMigrationCount(t, database, "migration_session_ddl", 0)
				}
				assertMigrationSession(t, pool, server.Settings.Database)

				writeMigration(t, path, "0002_session.sql", "INSERT INTO migration_items VALUES (2);")
				runMigrations(t, pool, path)
				assertMigrationVersion(t, database, 3)
				assertMigrationCount(t, database, "migration_items", 2)
				assertMigrationCount(t, database, "migration_next", 0)
			})
		}
	})

	t.Run("complete transactions succeed without leaking session settings", func(t *testing.T) {
		resetMigrationTables(t, database)
		pool := migrationPool(t, server.DSN, true, true)
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		path := t.TempDir()
		writeMigration(t, path, "0001_items.sql", "CREATE TABLE migration_items (id INT PRIMARY KEY) ENGINE=InnoDB;")
		writeMigration(t, path, "0002_transaction.sql", `
SET @margo_migration_test = 1;
SET SESSION foreign_key_checks = 0;
CREATE TEMPORARY TABLE migration_temporary (id INT PRIMARY KEY);
START TRANSACTION;
INSERT INTO migration_items VALUES (1), (2);
COMMIT;
`)
		writeMigration(t, path, "0003_items.sql", "INSERT INTO migration_items VALUES (3);")
		runMigrations(t, pool, path)
		assertMigrationVersion(t, database, 3)
		assertMigrationCount(t, database, "migration_items", 3)
		assertMigrationSession(t, pool, server.Settings.Database)
		_, err := pool.ExecContext(t.Context(), "SELECT * FROM migration_temporary")
		var mysqlError *mysql.MySQLError
		if !errors.As(err, &mysqlError) || mysqlError.Number != 1146 {
			t.Fatalf("temporary migration table leaked into caller pool: %v", err)
		}
	})

	t.Run("concurrent runs serialize and skip completed SQL", func(t *testing.T) {
		resetMigrationTables(t, database)
		path, release, first := blockedMigration(t, t.Context(), database)
		second := startMigration(t, t.Context(), database, path)
		select {
		case err := <-second:
			t.Fatalf("second runner returned while first held the migration lock: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		release()
		awaitMigration(t, first)
		awaitMigration(t, second)
		assertMigrationVersion(t, database, 1)
		assertMigrationCount(t, database, "migration_items", 2)
		runMigrations(t, database, path)
	})

	t.Run("cancel while waiting for lock", func(t *testing.T) {
		resetMigrationTables(t, database)
		path, release, first := blockedMigration(t, t.Context(), database)
		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()
		err := migrate.Run(ctx, migrate.Options{DB: database, Path: path})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected cancellation while acquiring migration lock, got %v", err)
		}
		release()
		awaitMigration(t, first)
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 1)
	})

	t.Run("cancel during SQL and retry", func(t *testing.T) {
		resetMigrationTables(t, database)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		path, release, first := blockedMigration(t, ctx, database)
		cancel()
		select {
		case err := <-first:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation during migration SQL, got %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("migration did not return after cancellation")
		}
		release()
		assertMigrationVersion(t, database, 0)

		const retrySafeSQL = `
CREATE TABLE IF NOT EXISTS migration_items (id INT PRIMARY KEY);
INSERT IGNORE INTO migration_items VALUES (1);
`
		writeMigration(t, path, "0001_items.sql", retrySafeSQL+"INSERT IGNORE INTO migration_items VALUES (2);")
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 1)
		assertMigrationCount(t, database, "migration_items", 2)
	})

	t.Run("lock timeout and release", func(t *testing.T) {
		resetMigrationTables(t, database)
		path, release, first := blockedMigration(t, t.Context(), database)
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
		defer cancel()
		err := migrate.Run(ctx, migrate.Options{DB: database, Path: path})
		if !errors.Is(err, errs.ErrLockTimeout) {
			t.Fatalf("expected 30-second advisory-lock timeout, got %v", err)
		}
		assertMigrationVersion(t, database, 0)
		release()
		awaitMigration(t, first)
		runMigrations(t, database, path)
		assertMigrationVersion(t, database, 1)
	})

	if err := database.PingContext(t.Context()); err != nil {
		t.Fatalf("migrator closed caller-owned pool: %v", err)
	}
}

func migrationPool(t testing.TB, dsn string, multiStatements, selected bool) *sql.DB {
	t.Helper()
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse migration DSN: %v", err)
	}
	config.MultiStatements = multiStatements
	if !selected {
		config.DBName = ""
	}
	database, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatalf("open migration pool: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close migration pool: %v", err)
		}
	})
	return database
}

func resetMigrationTables(t testing.TB, database *sql.DB) {
	t.Helper()
	migrationExec(t, database, "DROP TABLE IF EXISTS "+migrate.TableName+", migration_items, migration_next, migration_session_ddl")
}

func writeMigration(t testing.TB, path, name, statement string) {
	t.Helper()
	writeTestFile(t, filepath.Join(path, name), []byte(statement))
}

func migrationExec(t testing.TB, database *sql.DB, statement string) {
	t.Helper()
	if _, err := database.ExecContext(t.Context(), statement); err != nil {
		t.Fatalf("execute migration test setup: %v", err)
	}
}

func runMigrations(t testing.TB, database *sql.DB, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, migrate.Options{DB: database, Path: path}); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func assertMigrationVersion(t testing.TB, database *sql.DB, want uint64) {
	t.Helper()
	var version uint64
	var rows int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*), COALESCE(MAX(version), 0) FROM "+migrate.TableName).Scan(&rows, &version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if rows != 1 || version != want {
		t.Fatalf("expected one version row at %d, got %d rows at %d", want, rows, version)
	}
}

func assertMigrationCount(t testing.TB, database *sql.DB, table string, want int) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("expected %d rows in %s, got %d", want, table, count)
	}
}

func assertMigrationTableAbsent(t testing.TB, database *sql.DB, table string) {
	t.Helper()
	var count int
	if err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table).Scan(&count); err != nil {
		t.Fatalf("check for table %s: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("pending migration unexpectedly created %s", table)
	}
}

func assertMigrationSession(t testing.TB, database *sql.DB, wantDatabase string) {
	t.Helper()
	var selected string
	var autocommit, inTransaction, foreignKeyChecks int
	var userVariable sql.NullInt64
	err := database.QueryRowContext(t.Context(), "SELECT DATABASE(), @@autocommit, @@in_transaction, @@foreign_key_checks, @margo_migration_test").Scan(
		&selected, &autocommit, &inTransaction, &foreignKeyChecks, &userVariable,
	)
	if err != nil {
		t.Fatalf("inspect caller-owned pool session: %v", err)
	}
	if selected != wantDatabase || autocommit != 1 || inTransaction != 0 || foreignKeyChecks != 1 || userVariable.Valid {
		t.Fatalf("migration session leaked into caller pool: database=%q autocommit=%d transaction=%d foreign_key_checks=%d user_variable=%v",
			selected, autocommit, inTransaction, foreignKeyChecks, userVariable)
	}
}

func assertMigrationFailure(t testing.TB, err error, filename string, code uint16) {
	t.Helper()
	if !errors.Is(err, errs.ErrMigrationFailed) {
		t.Fatalf("expected migration failure, got %v", err)
	}
	if !strings.Contains(err.Error(), filename) {
		t.Fatalf("migration failure does not identify %s: %v", filename, err)
	}
	var mysqlError *mysql.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != code {
		t.Fatalf("expected wrapped MariaDB error %d, got %v", code, err)
	}
}

// The first runner blocks inside its migration on a separate test-owned lock.
// A committed row confirms it holds the migration lock before another run starts.
func blockedMigration(t testing.TB, ctx context.Context, database *sql.DB) (string, func(), <-chan error) {
	t.Helper()
	const gate = "margo_integration_migration_gate"
	connection, err := database.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var acquired int
	if err := connection.QueryRowContext(t.Context(), "SELECT GET_LOCK(?, 0)", gate).Scan(&acquired); err != nil || acquired != 1 {
		_ = connection.Close()
		t.Fatalf("acquire migration test gate: acquired=%d, err=%v", acquired, err)
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := connection.ExecContext(ctx, "DO RELEASE_LOCK(?)", gate); err != nil {
			t.Errorf("release migration test gate: %v", err)
		}
		if err := connection.Close(); err != nil {
			t.Errorf("close migration test gate connection: %v", err)
		}
	}
	t.Cleanup(release)
	path := t.TempDir()
	writeMigration(t, path, "0001_items.sql", `
CREATE TABLE migration_items (id INT PRIMARY KEY);
INSERT INTO migration_items VALUES (1);
DO GET_LOCK('`+gate+`', 60);
DO RELEASE_LOCK('`+gate+`');
INSERT INTO migration_items VALUES (2);
`)
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	t.Cleanup(cancel)
	result := startMigration(t, ctx, database, path)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM migration_items").Scan(&count)
		if err == nil && count == 1 {
			return path, release, result
		}
		select {
		case err := <-result:
			t.Fatalf("first migration returned before blocking: %v", err)
		case <-deadline.C:
			t.Fatal("first migration did not reach the test gate")
		case <-ticker.C:
		}
	}
}

func startMigration(t testing.TB, ctx context.Context, database *sql.DB, path string) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		// The suite timeout bounds a broken cancellation path. Do not let the
		// next subtest reset shared tables while this runner is still active.
		<-done
	})
	go func() {
		defer close(done)
		result <- migrate.Run(ctx, migrate.Options{DB: database, Path: path})
	}()
	return result
}

func awaitMigration(t testing.TB, result <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("concurrent migration failed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("migration did not return after test gate was released")
	}
}
