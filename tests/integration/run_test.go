//go:build integration

package integration

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/runner"
)

// These tests import the public runner API from a separate package, as a consumer
// does. SQL remains path-based after extraction from the embedded fixture.
//
//go:embed testdata/migrations/*.sql
var embeddedMigrations embed.FS

func TestRunOperations(t *testing.T) {
	server := StartMariaDB(t)
	pool := migrationPool(t, server.DSN, true, true)
	pool.SetMaxOpenConns(3)
	queries := t.TempDir()
	writeTestFile(t, filepath.Join(queries, "GetIDs.sql"), []byte("-- Returns: id\nSELECT id FROM all_types;\n"))
	migrations := t.TempDir()
	writeMigration(t, migrations, "0001_items.sql", "CREATE TABLE api_items (id INT PRIMARY KEY); INSERT INTO api_items VALUES (1);")

	for _, owned := range []bool{false, true} {
		mode := "borrowed"
		if owned {
			mode = "owned"
		}
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name                        string
				output, queries, migrations bool
				want                        error
			}{
				{name: "no paths"},
				{name: "output", output: true},
				{name: "output and queries", output: true, queries: true},
				{name: "migrations", migrations: true},
				{name: "migrations and output", migrations: true, output: true},
				{name: "all paths", migrations: true, output: true, queries: true},
				{name: "queries without output", queries: true, want: errs.ErrQueriesWithoutOutput},
				{name: "migrations and queries without output", migrations: true, queries: true, want: errs.ErrQueriesWithoutOutput},
			} {
				t.Run(tc.name, func(t *testing.T) {
					migrationExec(t, server.DB, "DROP TABLE IF EXISTS api_items, "+migrate.TableName)
					module := newMigrationOutputModule(t)
					before := snapshotMigrationOutput(t, module)
					opts := runner.Options{DB: pool}
					if owned {
						opts.DB = nil
						opts.Connection = &server.Settings
					}
					if tc.output {
						opts.OutputPath = filepath.Join(module, "generated")
					}
					if tc.queries {
						opts.QueriesPath = queries
					}
					if tc.migrations {
						opts.MigrationsPath = migrations
					}
					if err := runner.Run(t.Context(), opts); !errors.Is(err, tc.want) {
						t.Fatalf("Run() = %v, want %v", err, tc.want)
					}
					migrated := tc.migrations && tc.want == nil
					assertCLIMigrationTable(t, server.DB, "api_items", migrated)
					if migrated {
						assertCLIMigrationVersion(t, server.DB, 1)
					}
					if tc.output {
						root := filepath.Join(opts.OutputPath, "MargoTest")
						content, err := os.ReadFile(filepath.Join(root, "queries.go"))
						if err != nil {
							t.Fatal(err)
						}
						if strings.Contains(string(content), "func QueryGetIDs(") != tc.queries {
							t.Errorf("named query presence does not match QueriesPath")
						}
						if _, err := os.Stat(filepath.Join(root, "AllTypes", "entity.go")); err != nil {
							t.Fatal(err)
						}
						if migrated {
							if _, err := os.Stat(filepath.Join(root, "ApiItems", "entity.go")); err != nil {
								t.Fatalf("generation did not inspect migrated schema: %v", err)
							}
						}
						assertNoMigrationBindings(t, opts.OutputPath)
					} else if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
						t.Error("Run without output modified files")
					}
					if err := pool.PingContext(t.Context()); err != nil {
						t.Fatalf("borrowed pool is no longer usable: %v", err)
					}
					if got := pool.Stats().MaxOpenConnections; got != 3 {
						t.Errorf("borrowed pool configuration changed to %d", got)
					}
				})
			}
		})
	}
}

func TestRunBorrowedErrors(t *testing.T) {
	server := StartMariaDB(t)
	pool := migrationPool(t, server.DSN, true, true)
	migrations := t.TempDir()
	writeMigration(t, migrations, "0001_broken.sql", "INSERT INTO api_missing_table VALUES (1);")

	t.Run("migration failure preserves output", func(t *testing.T) {
		for _, missing := range []bool{false, true} {
			module := newMigrationOutputModule(t)
			output := module
			if missing {
				output = filepath.Join(module, "missing", "generated")
			}
			before := snapshotMigrationOutput(t, module)
			err := runner.Run(t.Context(), runner.Options{DB: pool, OutputPath: output, MigrationsPath: migrations})
			assertMigrationFailure(t, err, "0001_broken.sql", 1146)
			if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
				t.Error("failed migration changed output or created directories")
			}
		}
	})

	t.Run("no selected database", func(t *testing.T) {
		unselected := migrationPool(t, server.DSN, true, false)
		module := newMigrationOutputModule(t)
		before := snapshotMigrationOutput(t, module)
		err := runner.Run(t.Context(), runner.Options{DB: unselected, OutputPath: filepath.Join(module, "missing")})
		if !errors.Is(err, errs.ErrDatabaseNotSelected) {
			t.Fatalf("Run() = %v, want no selected database", err)
		}
		if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
			t.Error("unselected database created output")
		}
		if err := unselected.PingContext(t.Context()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("migrator requirements remain inspectable", func(t *testing.T) {
		withoutMulti := migrationPool(t, server.DSN, false, true)
		err := runner.Run(t.Context(), runner.Options{DB: withoutMulti, MigrationsPath: migrations})
		if !errors.Is(err, errs.ErrMultiStatementsRequired) {
			t.Fatalf("Run() = %v, want multi-statement requirement", err)
		}
		if err := withoutMulti.PingContext(t.Context()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("owned failures close sessions", func(t *testing.T) {
		count := func(t *testing.T) int {
			t.Helper()
			var n int
			// Include server-only bootstrap connections, which have no selected DB.
			if err := server.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.processlist WHERE USER = ?", server.Settings.User).Scan(&n); err != nil {
				t.Fatal(err)
			}
			return n
		}
		assertClosed := func(t *testing.T, before int) {
			t.Helper()
			deadline := time.Now().Add(3 * time.Second)
			for count(t) > before {
				if time.Now().After(deadline) {
					t.Fatal("owned connection remained open after failure")
				}
				time.Sleep(10 * time.Millisecond)
			}
		}

		t.Run("migration", func(t *testing.T) {
			before := count(t)
			err := runner.Run(t.Context(), runner.Options{Connection: &server.Settings, MigrationsPath: migrations})
			assertMigrationFailure(t, err, "0001_broken.sql", 1146)
			assertClosed(t, before)
		})

		t.Run("generation", func(t *testing.T) {
			output := newMigrationOutputModule(t)
			conflict := filepath.Join(output, "MargoTest")
			const original = "preserve the conflicting output file\n"
			writeTestFile(t, conflict, []byte(original))
			before := count(t)
			err := runner.Run(t.Context(), runner.Options{Connection: &server.Settings, OutputPath: output})
			var pathError *os.PathError
			if !errors.As(err, &pathError) || pathError.Path != conflict {
				t.Fatalf("expected inspectable output child failure, got %v", err)
			}
			content, readErr := os.ReadFile(conflict)
			if readErr != nil || string(content) != original {
				t.Fatalf("generation changed conflicting file: content=%q err=%v", content, readErr)
			}
			assertClosed(t, before)
		})

		t.Run("bootstrap", func(t *testing.T) {
			connection := server.Settings
			// The disposable fixture user only has grants on margo_test.
			connection.Database = "margo_forbidden"
			before := count(t)
			err := runner.Run(t.Context(), runner.Options{Connection: &connection, MigrationsPath: migrations})
			var databaseError *mysql.MySQLError
			if !errors.As(err, &databaseError) || databaseError.Number != 1044 {
				t.Fatalf("expected inspectable bootstrap permission failure, got %v", err)
			}
			assertClosed(t, before)
		})
	})

	t.Run("cancellation leaves pool usable", func(t *testing.T) {
		path := t.TempDir()
		writeMigration(t, path, "0001_slow.sql", "DO SLEEP(10);")
		ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
		defer cancel()
		err := runner.Run(ctx, runner.Options{DB: pool, MigrationsPath: path})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run() = %v, want deadline exceeded", err)
		}
		if err := pool.PingContext(t.Context()); err != nil {
			t.Fatalf("borrowed pool closed after cancellation: %v", err)
		}
	})

	if err := pool.PingContext(t.Context()); err != nil {
		t.Fatalf("borrowed pool closed after failure: %v", err)
	}
}

func TestRunEmbeddedBootstrap(t *testing.T) {
	server := StartMariaDB(t)
	migrationExec(t, server.DB, "DROP DATABASE `margo_test`")
	if err := server.DB.Close(); err != nil {
		t.Fatal(err)
	}
	// Generation alone must not recreate a missing database.
	err := runner.Run(t.Context(), runner.Options{Connection: &server.Settings, OutputPath: newMigrationOutputModule(t)})
	var databaseError *mysql.MySQLError
	if !errors.As(err, &databaseError) || databaseError.Number != 1049 {
		t.Fatalf("generation without migrations should retain unknown-database error: %v", err)
	}
	path := t.TempDir()
	files, err := fs.ReadDir(embeddedMigrations, "testdata/migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range files {
		content, err := embeddedMigrations.ReadFile("testdata/migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(path, entry.Name()), content)
	}
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	before := snapshotMigrationOutput(t, workingDir)
	connection := server.Settings
	original := connection
	if err := runner.Run(t.Context(), runner.Options{Connection: &connection, MigrationsPath: path}); err != nil {
		t.Fatal(err)
	}
	if connection != original {
		t.Error("Run changed caller connection options")
	}
	if after := snapshotMigrationOutput(t, workingDir); !maps.Equal(before, after) {
		t.Error("migration-only Run wrote files outside a Go module")
	}
	pool := migrationPool(t, server.DSN, true, true)
	assertCLIMigrationVersion(t, pool, 1)
	assertCLIMigrationTable(t, pool, "embedded_items", true)
}

func TestRunIndependentCalls(t *testing.T) {
	first := startMariaDB(t, "first_schema")
	second := startMariaDB(t, "second_schema")
	servers := []*MariaDB{first, second}
	queries := t.TempDir()
	writeTestFile(t, filepath.Join(queries, "GetIDs.sql"), []byte("-- Returns: id\nSELECT id FROM all_types;\n"))
	outputs := []string{newMigrationOutputModule(t), newMigrationOutputModule(t)}
	for i, server := range servers {
		opts := runner.Options{Connection: &server.Settings, OutputPath: outputs[i]}
		if i == 0 {
			opts.QueriesPath = queries
		}
		if err := runner.Run(t.Context(), opts); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotMigrationOutput(t, outputs[0])
	invalid := second.Settings
	invalid.Password = "deliberately-invalid"
	err := runner.Run(t.Context(), runner.Options{Connection: &invalid, OutputPath: newMigrationOutputModule(t)})
	if err == nil {
		t.Fatal("Run retained prior credentials instead of rejecting the supplied password")
	}
	for i, name := range []string{"FirstSchema", "SecondSchema"} {
		content, err := os.ReadFile(filepath.Join(outputs[i], name, "queries.go"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "func QueryGetIDs(") != (i == 0) {
			t.Error("repeated Run retained previous query path")
		}
	}
	if after := snapshotMigrationOutput(t, outputs[0]); !maps.Equal(before, after) {
		t.Error("later Run modified previous output")
	}

	var wg sync.WaitGroup
	results := make(chan error, 4)
	for i := range 4 {
		connection := servers[i%len(servers)].Settings
		output := newMigrationOutputModule(t)
		wg.Go(func() {
			results <- runner.Run(t.Context(), runner.Options{Connection: &connection, OutputPath: output})
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("concurrent Run: %v", err)
		}
	}
}

func TestRunCLIEquivalenceAndRuntime(t *testing.T) {
	server := StartMariaDB(t)
	cliOutput := newRunRuntimeModule(t)
	packageOutput := newRunRuntimeModule(t)
	queries := filepath.Join(margoRepositoryDir(t), "tests", "integration", "testdata", "queries")
	GenerateInto(t, server, cliOutput, queries)
	if err := runner.Run(t.Context(), runner.Options{DB: server.DB, OutputPath: packageOutput, QueriesPath: queries}); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(snapshotMigrationOutput(t, cliOutput), snapshotMigrationOutput(t, packageOutput)) {
		t.Error("CLI and package generated different files for identical schemas and module names")
	}
	writeTestFile(t, filepath.Join(packageOutput, "MargoTest", "runtime_test.go"), generatedRuntimeTests)
	runTestCommand(t, packageOutput, append(os.Environ(), "GOWORK=off", "MARGO_INTEGRATION_DSN="+server.DSN),
		"go", "test", "-mod=mod", "-tags=margo_generated_runtime", "-count=1", "-race", "-cover", "-covermode=atomic", "-p=1", "./...")
}

func TestRunLeavesProcessStateUntouched(t *testing.T) {
	server := StartMariaDB(t)
	output := newMigrationOutputModule(t)
	args := slices.Clone(os.Args)
	flags := flag.CommandLine
	flagValues := func() map[string]string {
		values := make(map[string]string)
		flag.CommandLine.VisitAll(func(f *flag.Flag) { values[f.Name] = f.Value.String() })
		return values
	}
	originalFlags := flagValues()
	logger := slog.Default()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	capture, err := os.CreateTemp(t.TempDir(), "stdio")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = capture.Close() })
	os.Stdout, os.Stderr = capture, capture
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	// Public generation must work inside an application that has no Go toolchain
	// or MarGO executable on PATH.
	t.Setenv("PATH", t.TempDir())
	if err := runner.Run(t.Context(), runner.Options{DB: server.DB, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(os.Args, args) || flag.CommandLine != flags || !maps.Equal(originalFlags, flagValues()) || slog.Default() != logger {
		t.Error("Run changed process arguments, flags, or default logger")
	}
	if got, err := os.Getwd(); err != nil || got != cwd {
		t.Errorf("Run changed working directory: %q, %v", got, err)
	}
	if os.Stdout != capture || os.Stderr != capture {
		t.Error("Run replaced standard output or error")
	}
	info, err := capture.Stat()
	if err != nil || info.Size() != 0 {
		t.Errorf("Run wrote standard output or error: info=%v err=%v", info, err)
	}
}

func TestRunEmptySchema(t *testing.T) {
	server := StartMariaDB(t)
	migrationExec(t, server.DB, "DROP TABLE all_types, alpha, beta")
	migrationExec(t, server.DB, "CREATE TABLE "+migrate.TableName+" (id INT PRIMARY KEY, version BIGINT)")
	output := newMigrationOutputModule(t)
	if err := runner.Run(t.Context(), runner.Options{DB: server.DB, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "MargoTest", "queries.go")); err != nil {
		t.Fatalf("empty schema did not produce database queries.go: %v", err)
	}
	assertNoMigrationBindings(t, output)
}

func assertNoMigrationBindings(t *testing.T, output string) {
	t.Helper()
	for path, content := range snapshotMigrationOutput(t, output) {
		if strings.Contains(path, "MargoSchemaVersion") || strings.Contains(content, migrate.TableName) {
			t.Errorf("internal migration table leaked into %s", path)
		}
	}
}

func newRunRuntimeModule(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	writeTestFile(t, filepath.Join(path, "go.mod"), []byte("module "+generatedTestModule+"\n\ngo 1.27\n\nrequire github.com/go-sql-driver/mysql v1.10.1\n"))
	return path
}
