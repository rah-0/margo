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
	"testing/fstest"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/tests/integration/testdata/migrations"
	queryfixture "github.com/rah-0/margo/tests/integration/testdata/queries"
)

// These tests import the public runner API from a separate package, as a consumer
// does. This parent filesystem also verifies selection through fs.Sub.
//
//go:embed testdata/migrations/*.sql
var embeddedMigrations embed.FS

//go:embed testdata/queries/*.sql
var embeddedQueries embed.FS

type runOperationCase struct {
	name                        string
	output, queries, migrations bool
	filesystem                  bool
	queriesFilesystem           bool
	want                        error
}

type runQuerySourceValidationCase struct {
	name   string
	source fs.FS
	output bool
	want   error
}

type runQueryFilesystemCase struct {
	name    string
	source  fs.FS
	owned   bool
	runtime bool
}

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
			for _, tc := range []runOperationCase{
				{name: "no paths"},
				{name: "output", output: true},
				{name: "output and queries", output: true, queries: true},
				{name: "output and query filesystem", output: true, queries: true, queriesFilesystem: true},
				{name: "migrations", migrations: true},
				{name: "migrations and output", migrations: true, output: true},
				{name: "all paths", migrations: true, output: true, queries: true},
				{name: "filesystem", migrations: true, filesystem: true},
				{name: "filesystem and output", migrations: true, filesystem: true, output: true},
				{name: "filesystem and all paths", migrations: true, filesystem: true, output: true, queries: true},
				{name: "both filesystems and output", migrations: true, filesystem: true, output: true, queries: true, queriesFilesystem: true},
				{name: "queries without output", queries: true, want: errs.ErrQueriesWithoutOutput},
				{name: "migrations and queries without output", migrations: true, queries: true, want: errs.ErrQueriesWithoutOutput},
				{name: "filesystem and queries without output", migrations: true, filesystem: true, queries: true, want: errs.ErrQueriesWithoutOutput},
				{name: "query filesystem without output", queries: true, queriesFilesystem: true, want: errs.ErrQueriesWithoutOutput},
				{name: "both filesystems without output", migrations: true, filesystem: true, queries: true, queriesFilesystem: true, want: errs.ErrQueriesWithoutOutput},
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
						if tc.queriesFilesystem {
							opts.Inputs.Queries = fstest.MapFS{
								"GetIDs.sql": {Data: []byte("-- Returns: id\nSELECT id FROM all_types;\n")},
							}
						} else {
							opts.Inputs.Queries = os.DirFS(queries)
						}
					}
					if tc.migrations {
						if tc.filesystem {
							opts.Inputs.Migrations = fstest.MapFS{
								"0001_items.sql": {Data: []byte("CREATE TABLE api_items (id INT PRIMARY KEY); INSERT INTO api_items VALUES (1);")},
							}
						} else {
							opts.Inputs.Migrations = os.DirFS(migrations)
						}
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
							t.Errorf("named query presence does not match query source")
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

	t.Run("query source validation precedes migrations", func(t *testing.T) {
		rootError := &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}
		for _, tc := range []runQuerySourceValidationCase{
			{name: "unreadable root", source: queryRootErrorFS{rootError}, output: true, want: fs.ErrPermission},
			{name: "no output", source: queryfixture.Files, want: errs.ErrQueriesWithoutOutput},
		} {
			t.Run(tc.name, func(t *testing.T) {
				migrationExec(t, pool, "DROP TABLE IF EXISTS api_preflight, "+migrate.TableName)
				module := newMigrationOutputModule(t)
				before := snapshotMigrationOutput(t, module)
				opts := runner.Options{
					DB: pool,
					Inputs: runner.Inputs{
						Queries: tc.source,
						Migrations: fstest.MapFS{
							"0001_preflight.sql": {Data: []byte("CREATE TABLE api_preflight (id INT PRIMARY KEY);")},
						},
					},
				}
				if tc.output {
					opts.OutputPath = filepath.Join(module, "missing", "generated")
				}
				err := runner.Run(t.Context(), opts)
				if !errors.Is(err, tc.want) {
					t.Fatalf("query preflight = %v, want %v", err, tc.want)
				}
				if tc.want == fs.ErrPermission {
					var pathError *fs.PathError
					if !errors.As(err, &pathError) || pathError != rootError {
						t.Fatalf("query root error lost filesystem cause: %v", err)
					}
				}
				assertMigrationTableAbsent(t, pool, "api_preflight")
				assertMigrationTableAbsent(t, pool, migrate.TableName)
				if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
					t.Error("query source validation changed output or created directories")
				}
			})
		}
	})

	t.Run("migration failure preserves output", func(t *testing.T) {
		for _, missing := range []bool{false, true} {
			module := newMigrationOutputModule(t)
			output := module
			if missing {
				output = filepath.Join(module, "missing", "generated")
			}
			before := snapshotMigrationOutput(t, module)
			err := runner.Run(t.Context(), runner.Options{DB: pool, OutputPath: output, Inputs: runner.Inputs{Migrations: os.DirFS(migrations)}})
			assertMigrationFailure(t, err, "0001_broken.sql", 1146)
			if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
				t.Error("failed migration changed output or created directories")
			}
		}
	})

	t.Run("migration failure leaves embedded query bodies unread", func(t *testing.T) {
		for _, missing := range []bool{false, true} {
			module := newMigrationOutputModule(t)
			output := module
			if missing {
				output = filepath.Join(module, "missing", "generated")
			}
			before := snapshotMigrationOutput(t, module)
			source := &migrationReadFS{source: queryfixture.Files}
			err := runner.Run(t.Context(), runner.Options{
				DB: pool, OutputPath: output,
				Inputs: runner.Inputs{
					Queries: source,
					Migrations: fstest.MapFS{
						"0001_broken.sql": {Data: []byte("INSERT INTO api_missing_table VALUES (1);")},
					},
				},
			})
			assertMigrationFailure(t, err, "0001_broken.sql", 1146)
			if len(source.bodies) != 0 {
				t.Errorf("query bodies read before migrations succeeded: %v", source.bodies)
			}
			assertMigrationFSOwnership(t, source)
			if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
				t.Error("failed embedded migration changed output or created directories")
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
		err := runner.Run(t.Context(), runner.Options{DB: withoutMulti, Inputs: runner.Inputs{Migrations: os.DirFS(migrations)}})
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
			err := runner.Run(t.Context(), runner.Options{Connection: &server.Settings, Inputs: runner.Inputs{Migrations: os.DirFS(migrations)}})
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
			err := runner.Run(t.Context(), runner.Options{Connection: &connection, Inputs: runner.Inputs{Migrations: os.DirFS(migrations)}})
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
		err := runner.Run(ctx, runner.Options{DB: pool, Inputs: runner.Inputs{Migrations: os.DirFS(path)}})
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

func TestRunQueryReadFailureAfterMigrations(t *testing.T) {
	server := StartMariaDB(t)
	pool := migrationPool(t, server.DSN, true, true)
	for _, missing := range []bool{false, true} {
		name := "existing output"
		if missing {
			name = "missing output"
		}
		t.Run(name, func(t *testing.T) {
			migrationExec(t, pool, "DROP TABLE IF EXISTS api_query_failure, "+migrate.TableName)
			module := newMigrationOutputModule(t)
			output := module
			if missing {
				output = filepath.Join(module, "missing", "generated")
			} else {
				for _, directory := range []string{"MargoTest", "errs"} {
					if err := os.MkdirAll(filepath.Join(output, directory), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				writeTestFile(t, filepath.Join(output, "MargoTest", "queries.go"), []byte("preserve existing queries\n"))
				writeTestFile(t, filepath.Join(output, "errs", "errors.go"), []byte("preserve existing errors\n"))
			}
			before := snapshotMigrationOutput(t, module)
			cause := &fs.PathError{Op: "read", Path: "BUnreadable.sql", Err: fs.ErrPermission}
			source := &migrationReadFS{
				source: fstest.MapFS{
					"AReady.sql":      {Data: []byte("-- Returns: id\nSELECT id FROM api_query_failure;")},
					"BUnreadable.sql": {},
					"CLater.sql":      {},
				},
				faults: map[string]error{"BUnreadable.sql": cause},
			}
			err := runner.Run(t.Context(), runner.Options{
				DB: pool, OutputPath: output,
				Inputs: runner.Inputs{
					Migrations: fstest.MapFS{
						"0001_items.sql": {Data: []byte("CREATE TABLE api_query_failure (id INT PRIMARY KEY); INSERT INTO api_query_failure VALUES (1);")},
					},
					Queries: source,
				},
			})
			var pathError *fs.PathError
			if !errors.Is(err, fs.ErrPermission) || !errors.As(err, &pathError) || pathError != cause || !strings.Contains(err.Error(), "BUnreadable.sql") {
				t.Fatalf("query read failure lost filename or filesystem cause: %v", err)
			}
			if !slices.Equal(source.bodies, []string{"AReady.sql", "BUnreadable.sql"}) {
				t.Errorf("query reads continued after failure: %v", source.bodies)
			}
			assertMigrationVersion(t, pool, 1)
			assertMigrationCount(t, pool, "api_query_failure", 1)
			assertMigrationFSOwnership(t, source)
			if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
				t.Error("query read failure modified output or created directories")
			}
			if err := pool.PingContext(t.Context()); err != nil {
				t.Fatalf("query read failure closed the borrowed pool: %v", err)
			}
		})
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
	t.Run("invalid query sources do not bootstrap the database", func(t *testing.T) {
		serverPool := migrationPool(t, server.DSN, true, false)
		module := newMigrationOutputModule(t)
		before := snapshotMigrationOutput(t, module)
		opts := runner.Options{
			Connection: &server.Settings, OutputPath: filepath.Join(module, "missing"),
			Inputs: runner.Inputs{
				Migrations: migrations.Files,
				Queries:    queryRootErrorFS{&fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}},
			},
		}
		want := fs.ErrPermission
		if err := runner.Run(t.Context(), opts); !errors.Is(err, want) {
			t.Fatalf("query preflight before bootstrap = %v, want %v", err, want)
		}
		var count int
		if err := serverPool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", server.Settings.Database).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Error("invalid query source created the missing database")
		}
		if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
			t.Error("invalid query source created output")
		}
	})
	workingDir := t.TempDir()
	t.Chdir(workingDir)
	before := snapshotMigrationOutput(t, workingDir)
	connection := server.Settings
	original := connection
	if err := runner.Run(t.Context(), runner.Options{Connection: &connection, Inputs: runner.Inputs{Migrations: migrations.Files}}); err != nil {
		t.Fatal(err)
	}
	if connection != original {
		t.Error("Run changed caller connection options")
	}
	if after := snapshotMigrationOutput(t, workingDir); !maps.Equal(before, after) {
		t.Error("migration-only Run wrote files outside a Go module")
	}
	pool := migrationPool(t, server.DSN, true, true)
	assertCLIMigrationVersion(t, pool, 2)
	assertCLIMigrationTable(t, pool, "embedded_items", true)
	assertMigrationCount(t, pool, "embedded_items", 2)
	if err := runner.Run(t.Context(), runner.Options{Connection: &connection, Inputs: runner.Inputs{Migrations: migrations.Files}}); err != nil {
		t.Fatalf("rerun embedded migrations: %v", err)
	}
	assertCLIMigrationVersion(t, pool, 2)
	assertMigrationCount(t, pool, "embedded_items", 2)
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
			opts.Inputs.Queries = os.DirFS(queries)
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
	if err := runner.Run(t.Context(), runner.Options{DB: server.DB, OutputPath: packageOutput, Inputs: runner.Inputs{Queries: os.DirFS(queries)}}); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(snapshotMigrationOutput(t, cliOutput), snapshotMigrationOutput(t, packageOutput)) {
		t.Error("CLI and package generated different files for identical schemas and module names")
	}
	subdirectory, err := fs.Sub(embeddedQueries, "testdata/queries")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []runQueryFilesystemCase{
		{name: "embedded borrowed", source: queryfixture.Files, runtime: true},
		{name: "embedded owned", source: queryfixture.Files, owned: true},
		{name: "embedded subdirectory", source: subdirectory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := newRunRuntimeModule(t)
			opts := runner.Options{DB: server.DB, OutputPath: output, Inputs: runner.Inputs{Queries: tc.source}}
			if tc.owned {
				opts.DB = nil
				opts.Connection = &server.Settings
			}
			if err := runner.Run(t.Context(), opts); err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(snapshotMigrationOutput(t, cliOutput), snapshotMigrationOutput(t, output)) {
				t.Error("disk and embedded query sources generated different files")
			}
			if err := server.DB.PingContext(t.Context()); err != nil {
				t.Fatalf("borrowed pool is no longer usable: %v", err)
			}
			if tc.runtime {
				writeTestFile(t, filepath.Join(output, "MargoTest", "runtime_test.go"), generatedRuntimeTests)
				runTestCommand(t, output, append(os.Environ(), "GOWORK=off", "MARGO_INTEGRATION_DSN="+server.DSN),
					"go", "test", "-mod=mod", "-tags=margo_generated_runtime", "-count=1", "-race", "-cover", "-covermode=atomic", "-p=1", "./...")
			}
		})
	}
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

type queryRootErrorFS struct {
	err *fs.PathError
}

func (source queryRootErrorFS) Open(string) (fs.File, error) {
	return nil, source.err
}
