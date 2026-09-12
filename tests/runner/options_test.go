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

type connectionOwnershipCase struct {
	name string
	opts runner.Options
	want error
}

type outputValidationCase struct {
	name string
	opts runner.Options
	want error
}

type filesystemValidationCase struct {
	name    string
	source  fs.FS
	want    error
	queries bool
}

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
	for _, test := range []connectionOwnershipCase{
		{name: "missing generation connection", opts: runner.Options{OutputPath: t.TempDir()}, want: errs.ErrConnectionRequired},
		{name: "empty queries filesystem requires connection", opts: runner.Options{OutputPath: t.TempDir(), Inputs: runner.Inputs{Queries: fstest.MapFS{}}}, want: errs.ErrConnectionRequired},
		{name: "missing migration connection", opts: runner.Options{Inputs: runner.Inputs{Migrations: os.DirFS(t.TempDir())}}, want: errs.ErrConnectionRequired},
		{name: "empty filesystem enables migrations", opts: runner.Options{Inputs: runner.Inputs{Migrations: fstest.MapFS{}}}, want: errs.ErrConnectionRequired},
		{name: "filesystem requires connection", opts: runner.Options{Inputs: runner.Inputs{Migrations: fstest.MapFS{"0001_initial.sql": {}}}}, want: errs.ErrConnectionRequired},
		{name: "conflicting connections", opts: runner.Options{DB: &sql.DB{}, Connection: &structs.ConnectionOptions{}, OutputPath: t.TempDir()}, want: errs.ErrConnectionConflict},
		{name: "filesystem conflicting connections", opts: runner.Options{DB: &sql.DB{}, Connection: &structs.ConnectionOptions{}, Inputs: runner.Inputs{Migrations: fstest.MapFS{}}}, want: errs.ErrConnectionConflict},
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

func TestRunOptionsValidateOutputBeforeConnections(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []outputValidationCase{
		{name: "queries require output", opts: runner.Options{Inputs: runner.Inputs{Queries: os.DirFS(t.TempDir())}}, want: errs.ErrQueriesWithoutOutput},
		{name: "named queries require output", opts: runner.Options{Inputs: runner.Inputs{Queries: fstest.MapFS{"Find.sql": {}}}}, want: errs.ErrQueriesWithoutOutput},
		{name: "empty queries require output", opts: runner.Options{Inputs: runner.Inputs{Queries: fstest.MapFS{}}}, want: errs.ErrQueriesWithoutOutput},
		{name: "queries with migrations require output", opts: runner.Options{Inputs: runner.Inputs{Queries: fstest.MapFS{}, Migrations: fstest.MapFS{}}}, want: errs.ErrQueriesWithoutOutput},
		{name: "output file", opts: runner.Options{OutputPath: file}, want: errs.ErrOutputPathNotDir},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A zero-value pool panics if queried: validation must finish first.
			test.opts.DB = &sql.DB{}
			if err := runner.Run(t.Context(), test.opts); !errors.Is(err, test.want) {
				t.Fatalf("Run error = %v, want %v", err, test.want)
			}
		})
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
		{Connection: &connection, Inputs: runner.Inputs{Migrations: fstest.MapFS{}}},
		{DB: &sql.DB{}, Inputs: runner.Inputs{Migrations: fstest.MapFS{}}},
		{Connection: &connection, OutputPath: output, Inputs: runner.Inputs{Queries: fstest.MapFS{}}},
		{DB: &sql.DB{}, OutputPath: output, Inputs: runner.Inputs{Queries: fstest.MapFS{}}},
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

func TestRunOptionsValidateFSBeforeConnections(t *testing.T) {
	t.Parallel()
	cause := &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}
	for _, test := range []filesystemValidationCase{
		{name: "root is a file", source: fstest.MapFS{".": {Data: []byte("not a directory")}}},
		{name: "root is missing", source: os.DirFS(filepath.Join(t.TempDir(), "missing")), want: fs.ErrNotExist},
		{name: "root cannot be enumerated", source: &observedFS{err: cause}, want: cause},
		{name: "queries root is a file", source: fstest.MapFS{".": {Data: []byte("not a directory")}}, queries: true},
		{name: "queries root is missing", source: os.DirFS(filepath.Join(t.TempDir(), "missing")), want: fs.ErrNotExist, queries: true},
		{name: "queries root cannot be enumerated", source: &observedFS{err: cause}, want: cause, queries: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, ownership := range []string{"borrowed", "owned"} {
				t.Run(ownership, func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "output")
					opts := runner.Options{OutputPath: output, Inputs: runner.Inputs{Migrations: test.source}}
					if test.queries {
						opts.Inputs.Migrations = nil
						opts.Inputs.Queries = test.source
					}
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
	for _, opts := range []runner.Options{
		{DB: new(sql.DB), Inputs: runner.Inputs{Migrations: source}},
		{DB: new(sql.DB), OutputPath: t.TempDir(), Inputs: runner.Inputs{Queries: source}},
	} {
		if err := runner.Run(ctx, opts); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	}
	if source.opened != 0 {
		t.Fatalf("canceled run opened the filesystem %d times", source.opened)
	}
}

func TestRunOptionsCanceledDuringRootRead(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"queries", "migrations"} {
		t.Run(input, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			source := &observedFS{source: fstest.MapFS{}, afterOpen: cancel}
			next := &observedFS{source: fstest.MapFS{}}
			output := filepath.Join(t.TempDir(), "output")
			opts := runner.Options{DB: new(sql.DB), OutputPath: output}
			if input == "queries" {
				opts.Inputs = runner.Inputs{Queries: source, Migrations: next}
			} else {
				opts.Inputs.Migrations = source
			}
			if err := runner.Run(ctx, opts); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run error = %v, want cancellation before connection access", err)
			}
			if source.opened != 1 || next.opened != 0 {
				t.Fatalf("unexpected root reads: canceled=%d next=%d", source.opened, next.opened)
			}
			if _, err := os.Stat(output); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("canceled operation created output: %v", err)
			}
		})
	}
}

type observedFS struct {
	source    fs.FS
	err       error
	errPath   string
	opened    int
	afterOpen func()
}

func (s *observedFS) Open(name string) (fs.File, error) {
	s.opened++
	if s.err != nil && (s.errPath == "" || s.errPath == name) {
		return nil, s.err
	}
	file, err := s.source.Open(name)
	if s.afterOpen != nil {
		s.afterOpen()
	}
	return file, err
}
