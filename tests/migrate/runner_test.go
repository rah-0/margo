package migrate_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
)

func TestRunArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		opts migrate.Options
		want error
	}{
		{name: "missing database", want: errs.ErrDatabaseRequired},
		{name: "database before source conflict", opts: migrate.Options{Path: "missing", FS: fstest.MapFS{}}, want: errs.ErrDatabaseRequired},
		{name: "database before filesystem validation", opts: migrate.Options{FS: fstest.MapFS{".": {}}}, want: errs.ErrDatabaseRequired},
		{name: "missing source", opts: migrate.Options{DB: new(sql.DB)}, want: errs.ErrPathRequired},
		{name: "source conflict before discovery", opts: migrate.Options{DB: new(sql.DB), Path: "missing", FS: fstest.MapFS{}}, want: errs.ErrMigrationsSourceConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := migrate.Run(t.Context(), test.opts); !errors.Is(err, test.want) {
				t.Fatalf("Run error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRunInvalidDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, filepath.Join(dir, "missing")} {
		err := migrate.Run(t.Context(), migrate.Options{DB: new(sql.DB), Path: path})
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("Run(%q) error = %v, want filesystem error before database access", path, err)
		}
	}
}

func TestRunInvalidFilesBeforeDatabaseAccess(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "0001_valid.sql")
	writeMigration(t, dir, "invalid.sql")
	if err := migrate.Run(t.Context(), migrate.Options{DB: new(sql.DB), Path: dir}); !errors.Is(err, errs.ErrInvalidFilename) {
		t.Fatalf("Run error = %v, want ErrInvalidFilename before database access", err)
	}
	for _, source := range []fs.FS{
		fstest.MapFS{"0001_valid.sql": {}, "invalid.sql": {}},
		&openOnlyFS{source: fstest.MapFS{"0001_valid.sql": {}, "invalid.sql": {}}},
	} {
		if err := migrate.Run(t.Context(), migrate.Options{DB: new(sql.DB), FS: source}); !errors.Is(err, errs.ErrInvalidFilename) {
			t.Fatalf("Run filesystem error = %v, want ErrInvalidFilename before database access", err)
		}
	}
}

func TestRunInvalidFSBeforeDatabaseAccess(t *testing.T) {
	cause := &fs.PathError{Op: "open", Path: ".", Err: fs.ErrPermission}
	for _, test := range []struct {
		source fs.FS
		cause  *fs.PathError
	}{
		{source: fstest.MapFS{".": {Data: []byte("not a directory")}}},
		{source: &openOnlyFS{openErr: cause}, cause: cause},
	} {
		err := migrate.Run(t.Context(), migrate.Options{DB: new(sql.DB), FS: test.source})
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("Run error = %v, want filesystem error before database access", err)
		}
		if test.cause != nil && (!errors.Is(err, fs.ErrPermission) || pathErr != test.cause) {
			t.Fatalf("Run error = %v, want original filesystem cause", err)
		}
	}
}

func TestRunFSCancellationBoundaries(t *testing.T) {
	for _, stage := range []string{"before discovery", "after discovery"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			source := &openOnlyFS{source: fstest.MapFS{}}
			if stage == "before discovery" {
				cancel()
			} else {
				source.afterReadDir = cancel
			}
			if err := migrate.Run(ctx, migrate.Options{DB: new(sql.DB), FS: source}); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run error = %v, want cancellation before database access", err)
			}
			if stage == "before discovery" && len(source.opened) != 0 {
				t.Fatalf("canceled run opened the filesystem: %v", source.opened)
			}
		})
	}
}
