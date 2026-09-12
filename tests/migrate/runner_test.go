package migrate_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
)

func TestRunArguments(t *testing.T) {
	if err := migrate.Run(t.Context(), migrate.Options{}); !errors.Is(err, errs.ErrDatabaseRequired) {
		t.Fatalf("Run error = %v, want ErrDatabaseRequired", err)
	}
	if err := migrate.Run(t.Context(), migrate.Options{DB: new(sql.DB)}); !errors.Is(err, errs.ErrPathRequired) {
		t.Fatalf("Run error = %v, want ErrPathRequired", err)
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
}
