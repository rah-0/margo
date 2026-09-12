//go:build integration

package integration

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/tests/integration/testdata/migrations"
)

func TestMigrationFilesystems(t *testing.T) {
	server := StartMariaDB(t)
	pool := migrationPool(t, server.DSN, true, true)
	pool.SetMaxOpenConns(3)

	t.Run("disk reads preserve symlink parent semantics", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "actual", "child")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		writeMigration(t, filepath.Dir(target), "0001_items.sql", "CREATE TABLE migration_items (id INT PRIMARY KEY); INSERT INTO migration_items VALUES (1);")
		writeMigration(t, root, "0001_items.sql", "INVALID SQL: the cleaned path must not be read")
		path := link + string(os.PathSeparator) + ".."
		for _, direct := range []bool{false, true} {
			resetMigrationTables(t, pool)
			var err error
			if direct {
				err = migrate.Run(t.Context(), migrate.Options{DB: pool, Path: path})
			} else {
				err = runner.Run(t.Context(), runner.Options{DB: pool, MigrationsPath: path})
			}
			if err != nil {
				t.Fatalf("run migrations through symlink parent: %v", err)
			}
			assertMigrationVersion(t, pool, 1)
			assertMigrationCount(t, pool, "migration_items", 1)
		}
	})

	t.Run("disk and embedded sources produce identical results", func(t *testing.T) {
		subdirectory, err := fs.Sub(embeddedMigrations, "testdata/migrations")
		if err != nil {
			t.Fatal(err)
		}
		queries := t.TempDir()
		writeTestFile(t, filepath.Join(queries, "GetDetail.sql"), []byte("-- Returns: detail\nSELECT detail FROM embedded_items WHERE id = ?;\n"))
		var wantOutput map[string]string
		for _, tc := range []struct {
			name   string
			source fs.FS
			direct bool
		}{
			{name: "disk"},
			{name: "embedded runner", source: migrations.Files},
			{name: "embedded migrate", source: migrations.Files, direct: true},
			{name: "embedded subdirectory", source: subdirectory},
		} {
			t.Run(tc.name, func(t *testing.T) {
				migrationExec(t, pool, "DROP TABLE IF EXISTS embedded_items, "+migrate.TableName)
				output := newMigrationOutputModule(t)
				opts := runner.Options{DB: pool, OutputPath: output, QueriesPath: queries}
				if tc.direct {
					if err := migrate.Run(t.Context(), migrate.Options{DB: pool, FS: tc.source}); err != nil {
						t.Fatal(err)
					}
				} else if tc.source != nil {
					opts.MigrationsFS = tc.source
				} else {
					opts.MigrationsPath = filepath.Join(margoRepositoryDir(t), "tests", "integration", "testdata", "migrations")
				}
				if err := runner.Run(t.Context(), opts); err != nil {
					t.Fatal(err)
				}
				assertMigrationVersion(t, pool, 2)
				rows, err := pool.QueryContext(t.Context(), "SELECT id, label, detail FROM embedded_items ORDER BY id")
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				type item struct {
					id            int
					label, detail string
				}
				var got []item
				for rows.Next() {
					var value item
					if err := rows.Scan(&value.id, &value.label, &value.detail); err != nil {
						t.Fatal(err)
					}
					got = append(got, value)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				want := []item{{1, "embedded", "migrated"}, {2, "second; embedded", "added"}}
				if !slices.Equal(got, want) {
					t.Fatalf("migrated rows = %+v, want %+v", got, want)
				}
				if tc.direct {
					if err := migrate.Run(t.Context(), migrate.Options{DB: pool, FS: tc.source}); err != nil {
						t.Fatalf("rerun embedded migrations: %v", err)
					}
				} else if err := runner.Run(t.Context(), opts); err != nil {
					t.Fatalf("rerun migrations and generation: %v", err)
				}
				assertMigrationVersion(t, pool, 2)
				assertMigrationCount(t, pool, "embedded_items", 2)
				if err := pool.PingContext(t.Context()); err != nil {
					t.Fatalf("borrowed pool is no longer usable: %v", err)
				}
				if got := pool.Stats().MaxOpenConnections; got != 3 {
					t.Errorf("borrowed pool configuration changed to %d", got)
				}
				entity, err := os.ReadFile(filepath.Join(output, "MargoTest", "EmbeddedItems", "entity.go"))
				if err != nil || !strings.Contains(string(entity), "Detail") {
					t.Fatalf("generation omitted migration-added field: %v", err)
				}
				assertNoMigrationBindings(t, output)
				gotOutput := snapshotMigrationOutput(t, output)
				if wantOutput == nil {
					wantOutput = gotOutput
				} else if !maps.Equal(gotOutput, wantOutput) {
					t.Error("disk and embedded migrations generated different output")
				}
			})
		}
	})

	t.Run("pending reads stop at failure and completed bodies stay unread", func(t *testing.T) {
		for _, mode := range []string{"migrate", "runner existing output", "runner missing output"} {
			t.Run(mode, func(t *testing.T) {
				resetMigrationTables(t, pool)
				module := newMigrationOutputModule(t)
				output := module
				if mode == "runner missing output" {
					output = filepath.Join(module, "missing", "generated")
				}
				before := snapshotMigrationOutput(t, module)
				readError := &fs.PathError{Op: "read", Path: "0002_items.sql", Err: fs.ErrPermission}
				source := &migrationReadFS{
					source: migrationReadFixture(),
					faults: map[string]error{"0002_items.sql": readError},
				}
				run := func() error {
					if mode == "migrate" {
						return migrate.Run(t.Context(), migrate.Options{DB: pool, FS: source})
					}
					return runner.Run(t.Context(), runner.Options{DB: pool, MigrationsFS: source, OutputPath: output})
				}
				err := run()
				var pathError *fs.PathError
				if !errors.Is(err, errs.ErrMigrationFailed) || !errors.Is(err, fs.ErrPermission) || !errors.As(err, &pathError) || pathError != readError {
					t.Fatalf("pending read error lost classification or cause: %v", err)
				}
				if !strings.Contains(err.Error(), "0002_items.sql") || !strings.Contains(err.Error(), "version 2") {
					t.Fatalf("pending read error lost filename or version: %v", err)
				}
				if !slices.Equal(source.bodies, []string{"0001_items.sql", "0002_items.sql"}) {
					t.Fatalf("read beyond failed migration: %v", source.bodies)
				}
				assertMigrationVersion(t, pool, 1)
				assertMigrationCount(t, pool, "migration_items", 1)
				assertMigrationTableAbsent(t, pool, "migration_next")
				if after := snapshotMigrationOutput(t, module); !maps.Equal(before, after) {
					t.Error("pending read failure changed output or created directories")
				}
				assertMigrationFSOwnership(t, source)

				// The caller can repair the source between synchronous calls. The
				// already checkpointed body's read error must never be observed.
				source.faults = map[string]error{"0001_items.sql": fs.ErrPermission}
				source.bodies = nil
				if err := run(); err != nil {
					t.Fatalf("resume after pending read failure: %v", err)
				}
				if !slices.Equal(source.bodies, []string{"0002_items.sql", "0003_next.sql"}) {
					t.Errorf("read completed bodies or skipped pending bodies: %v", source.bodies)
				}
				assertMigrationVersion(t, pool, 3)
				assertMigrationCount(t, pool, "migration_items", 2)
				assertMigrationCount(t, pool, "migration_next", 0)
				assertMigrationFSOwnership(t, source)

				source.faults["0002_items.sql"] = fs.ErrPermission
				source.faults["0003_next.sql"] = fs.ErrPermission
				source.bodies = nil
				if err := run(); err != nil {
					t.Fatalf("rerun with unreadable completed migrations: %v", err)
				}
				if len(source.bodies) != 0 {
					t.Errorf("read completed migration bodies: %v", source.bodies)
				}
				assertMigrationFSOwnership(t, source)
				if err := pool.PingContext(t.Context()); err != nil {
					t.Fatalf("borrowed pool closed after filesystem failure: %v", err)
				}
			})
		}
	})

	t.Run("empty filesystem still initializes migrations", func(t *testing.T) {
		for _, direct := range []bool{false, true} {
			resetMigrationTables(t, pool)
			var err error
			if direct {
				err = migrate.Run(t.Context(), migrate.Options{DB: pool, FS: fstest.MapFS{}})
			} else {
				err = runner.Run(t.Context(), runner.Options{DB: pool, MigrationsFS: fstest.MapFS{}})
			}
			if err != nil {
				t.Fatal(err)
			}
			assertMigrationVersion(t, pool, 0)
		}
	})

	t.Run("cancellation after a read stops before its SQL", func(t *testing.T) {
		for _, direct := range []bool{false, true} {
			resetMigrationTables(t, pool)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			source := &migrationReadFS{source: migrationReadFixture(), cancel: cancel, cancelName: "0002_items.sql"}
			var err error
			if direct {
				err = migrate.Run(ctx, migrate.Options{DB: pool, FS: source})
			} else {
				err = runner.Run(ctx, runner.Options{DB: pool, MigrationsFS: source})
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("read-boundary cancellation = %v", err)
			}
			assertMigrationVersion(t, pool, 1)
			assertMigrationCount(t, pool, "migration_items", 1)
			assertMigrationTableAbsent(t, pool, "migration_next")
			if !slices.Equal(source.bodies, []string{"0001_items.sql", "0002_items.sql"}) {
				t.Errorf("read migrations after cancellation: %v", source.bodies)
			}
			assertMigrationFSOwnership(t, source)
			if err := pool.PingContext(t.Context()); err != nil {
				t.Fatalf("borrowed pool closed after cancellation: %v", err)
			}
		}
	})
}

func migrationReadFixture() fs.FS {
	return fstest.MapFS{
		"0001_items.sql": {Data: []byte("CREATE TABLE migration_items (id INT PRIMARY KEY); INSERT INTO migration_items VALUES (1);")},
		"0002_items.sql": {Data: []byte("INSERT INTO migration_items VALUES (2);")},
		"0003_next.sql":  {Data: []byte("CREATE TABLE migration_next (id INT PRIMARY KEY);")},
	}
}

// Only Open is exposed from fs.FS, forcing ReadDir and ReadFile to use their
// standard fallbacks. Close belongs to the caller, while opened handles must close.
type migrationReadFS struct {
	source        fs.FS
	faults        map[string]error
	bodies        []string
	opens, closes int
	closed        bool
	cancelName    string
	cancel        context.CancelFunc
}

func (source *migrationReadFS) Open(name string) (fs.File, error) {
	file, err := source.source.Open(name)
	if err != nil {
		return nil, err
	}
	source.opens++
	if name != "." {
		source.bodies = append(source.bodies, name)
	}
	return &migrationReadFile{File: file, source: source, name: name}, nil
}

func (source *migrationReadFS) Close() error {
	source.closed = true
	return nil
}

type migrationReadFile struct {
	fs.File
	source *migrationReadFS
	name   string
}

func (file *migrationReadFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return file.File.(fs.ReadDirFile).ReadDir(n)
}

func (file *migrationReadFile) Read(p []byte) (int, error) {
	if err := file.source.faults[file.name]; err != nil {
		return 0, err
	}
	n, err := file.File.Read(p)
	if file.source.cancel != nil && file.name == file.source.cancelName {
		file.source.cancel()
	}
	return n, err
}

func (file *migrationReadFile) Close() error {
	file.source.closes++
	return file.File.Close()
}

func assertMigrationFSOwnership(t *testing.T, source *migrationReadFS) {
	t.Helper()
	if source.opens != source.closes {
		t.Errorf("filesystem handles leaked: opened %d, closed %d", source.opens, source.closes)
	}
	if source.closed {
		t.Error("migration runner closed the caller-owned filesystem")
	}
}
