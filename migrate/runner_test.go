package migrate

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunArguments(t *testing.T) {
	if err := Run(t.Context(), Options{}); !errors.Is(err, ErrDatabaseRequired) {
		t.Fatalf("Run error = %v, want ErrDatabaseRequired", err)
	}
	if err := Run(t.Context(), Options{DB: new(sql.DB)}); !errors.Is(err, ErrPathRequired) {
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
		err := Run(t.Context(), Options{DB: new(sql.DB), Path: path})
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
	if err := Run(t.Context(), Options{DB: new(sql.DB), Path: dir}); !errors.Is(err, ErrInvalidFilename) {
		t.Fatalf("Run error = %v, want ErrInvalidFilename before database access", err)
	}
}
