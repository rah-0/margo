package runner_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/structs"
)

func TestRunOptionsNoOpPrecedesValidation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, opts := range []runner.Options{
		{},
		{Connection: &structs.ConnectionOptions{}},
		{DB: &sql.DB{}},
		{DB: &sql.DB{}, Connection: &structs.ConnectionOptions{}},
	} {
		if err := runner.Run(ctx, opts); err != nil {
			t.Fatalf("no-op rejected: %v", err)
		}
	}
}

func TestRunOptionsConnectionOwnership(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		opts runner.Options
		want error
	}{
		{name: "missing generation connection", opts: runner.Options{OutputPath: t.TempDir()}, want: errs.ErrConnectionRequired},
		{name: "missing migration connection", opts: runner.Options{MigrationsPath: t.TempDir()}, want: errs.ErrConnectionRequired},
		{name: "empty filesystem enables migrations", opts: runner.Options{MigrationsFS: fstest.MapFS{}}, want: errs.ErrConnectionRequired},
		{name: "filesystem requires connection", opts: runner.Options{MigrationsFS: fstest.MapFS{"0001_initial.sql": {}}}, want: errs.ErrConnectionRequired},
		{name: "conflicting connections", opts: runner.Options{DB: &sql.DB{}, Connection: &structs.ConnectionOptions{}, OutputPath: t.TempDir()}, want: errs.ErrConnectionConflict},
		{name: "filesystem conflicting connections", opts: runner.Options{DB: &sql.DB{}, Connection: &structs.ConnectionOptions{}, MigrationsFS: fstest.MapFS{}}, want: errs.ErrConnectionConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := runner.Run(context.Background(), test.opts); !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func TestRunOptionsRequiresConnectionFields(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"User", "Password", "Host", "Database"} {
		t.Run(field, func(t *testing.T) {
			connection := structs.ConnectionOptions{User: "user", Password: "password", Host: "127.0.0.1", Database: "database"}
			switch field {
			case "User":
				connection.User = ""
			case "Password":
				connection.Password = ""
			case "Host":
				connection.Host = ""
			case "Database":
				connection.Database = ""
			}
			previous := connection
			err := runner.Run(context.Background(), runner.Options{Connection: &connection, OutputPath: t.TempDir()})
			if !errors.Is(err, errs.ErrInvalidConnection) || !strings.Contains(err.Error(), field) {
				t.Fatalf("expected missing %s connection error, got %v", field, err)
			}
			if connection != previous {
				t.Fatal("validation changed connection settings")
			}
		})
	}
}

func TestRunOptionsValidatePathsBeforeConnections(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	for _, test := range []struct {
		name    string
		opts    runner.Options
		want    error
		missing bool
	}{
		{name: "queries require output", opts: runner.Options{QueriesPath: dir}, want: errs.ErrQueriesWithoutOutput},
		{name: "queries with migrations require output", opts: runner.Options{QueriesPath: dir, MigrationsPath: dir}, want: errs.ErrQueriesWithoutOutput},
		{name: "queries with filesystem require output", opts: runner.Options{QueriesPath: dir, MigrationsFS: fstest.MapFS{}}, want: errs.ErrQueriesWithoutOutput},
		{name: "output file", opts: runner.Options{OutputPath: file}, want: errs.ErrOutputPathNotDir},
		{name: "queries file", opts: runner.Options{OutputPath: dir, QueriesPath: file}, want: errs.ErrQueriesPathNotDir},
		{name: "migrations file", opts: runner.Options{MigrationsPath: file}, want: errs.ErrMigrationsPathNotDir},
		{name: "missing queries", opts: runner.Options{OutputPath: dir, QueriesPath: missing}, want: errs.ErrQueriesPathInvalid, missing: true},
		{name: "missing migrations", opts: runner.Options{MigrationsPath: missing}, want: errs.ErrMigrationsPathInvalid, missing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A zero-value pool panics if queried: validation must finish first.
			test.opts.DB = &sql.DB{}
			err := runner.Run(context.Background(), test.opts)
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
			if test.missing {
				var pathErr *fs.PathError
				if !errors.Is(err, fs.ErrNotExist) || !errors.As(err, &pathErr) || pathErr.Path != missing {
					t.Fatalf("filesystem error is not inspectable: %v", err)
				}
			}
		})
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("validation created missing directory: %v", err)
	}
}

func TestRunOptionsCanceledBeforeConnection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	connection := structs.ConnectionOptions{User: "user", Password: "password", Host: "127.0.0.1", Database: "database"}
	previous := connection
	output := filepath.Join(t.TempDir(), "missing", "generated")
	for _, opts := range []runner.Options{
		{Connection: &connection, OutputPath: output},
		{DB: &sql.DB{}, OutputPath: output},
		{Connection: &connection, MigrationsFS: fstest.MapFS{}},
		{DB: &sql.DB{}, MigrationsFS: fstest.MapFS{}},
	} {
		if err := runner.Run(ctx, opts); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	}
	if connection != previous {
		t.Fatal("Run changed caller connection settings")
	}
	if _, err := os.Stat(filepath.Dir(output)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("canceled operation created output directories: %v", err)
	}
}

func TestRunOptionsConflictingMigrationSources(t *testing.T) {
	t.Parallel()
	for _, ownership := range []string{"borrowed", "owned", "missing connection"} {
		t.Run(ownership, func(t *testing.T) {
			source := &observedFS{err: &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}}
			output := filepath.Join(t.TempDir(), "output")
			opts := runner.Options{OutputPath: output, MigrationsPath: "missing", MigrationsFS: source}
			if ownership == "borrowed" {
				opts.DB = new(sql.DB)
			} else if ownership == "owned" {
				opts.Connection = &structs.ConnectionOptions{User: "user", Password: "password", Host: "127.0.0.1", Database: "database"}
			}
			if err := runner.Run(t.Context(), opts); !errors.Is(err, errs.ErrMigrationsSourceConflict) {
				t.Fatalf("Run error = %v, want ErrMigrationsSourceConflict before source or connection access", err)
			}
			if source.opened != 0 {
				t.Fatalf("conflict opened the filesystem %d times", source.opened)
			}
			if _, err := os.Stat(output); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("source conflict created output: %v", err)
			}
		})
	}
}

func TestRunOptionsValidateFSBeforeConnections(t *testing.T) {
	t.Parallel()
	cause := &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}
	for _, test := range []struct {
		name   string
		source fs.FS
		want   error
	}{
		{name: "root is a file", source: fstest.MapFS{".": {Data: []byte("not a directory")}}},
		{name: "root is missing", source: os.DirFS(filepath.Join(t.TempDir(), "missing")), want: fs.ErrNotExist},
		{name: "root cannot be enumerated", source: &observedFS{err: cause}, want: cause},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, ownership := range []string{"borrowed", "owned"} {
				t.Run(ownership, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "output")
					opts := runner.Options{OutputPath: output, MigrationsFS: test.source}
					if ownership == "borrowed" {
						opts.DB = new(sql.DB)
					} else {
						opts.Connection = &structs.ConnectionOptions{User: "user", Password: "password", Host: "127.0.0.1", Database: "database"}
					}
					err := runner.Run(t.Context(), opts)
					var pathErr *fs.PathError
					if !errors.As(err, &pathErr) || (test.want != nil && !errors.Is(err, test.want)) {
						t.Fatalf("Run error = %v, want inspectable root error %v before connection access", err, test.want)
					}
					if _, err := os.Stat(output); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("invalid filesystem created output: %v", err)
					}
				})
			}
		})
	}
}

func TestRunOptionsCanceledBeforeFSAccess(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	source := &observedFS{source: fstest.MapFS{}}
	if err := runner.Run(ctx, runner.Options{DB: new(sql.DB), MigrationsFS: source}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if source.opened != 0 {
		t.Fatalf("canceled run opened the filesystem %d times", source.opened)
	}
}

type observedFS struct {
	source fs.FS
	err    error
	opened int
}

func (s *observedFS) Open(name string) (fs.File, error) {
	s.opened++
	if s.err != nil {
		return nil, s.err
	}
	return s.source.Open(name)
}
