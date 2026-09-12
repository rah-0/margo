package conf_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/errs"
)

func TestValidatePathsWithoutPaths(t *testing.T) {
	t.Parallel()
	if err := conf.ValidatePaths("", "", ""); err != nil {
		t.Fatalf("no operation rejected: %v", err)
	}
}

func TestValidatePathsMigrationsWithoutOutput(t *testing.T) {
	t.Parallel()
	if err := conf.ValidatePaths("", "", t.TempDir()); err != nil {
		t.Fatalf("migration-only paths rejected: %v", err)
	}
}

func TestValidatePathsQueriesRequireOutput(t *testing.T) {
	t.Parallel()
	for _, migrations := range []string{"", t.TempDir()} {
		if err := conf.ValidatePaths("", t.TempDir(), migrations); !errors.Is(err, errs.ErrQueriesWithoutOutput) {
			t.Fatalf("expected output requirement for custom queries, got %v", err)
		}
	}
}

func TestValidatePathsQueriesPathInvalid(t *testing.T) {
	t.Parallel()
	err := conf.ValidatePaths(t.TempDir(), filepath.Join(t.TempDir(), "does-not-exist"), "")
	if !errors.Is(err, errs.ErrQueriesPathInvalid) {
		t.Fatalf("expected ErrQueriesPathInvalid, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestValidatePathsQueriesPathNotDir(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(file, []byte("-- noop"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := conf.ValidatePaths(t.TempDir(), file, ""); !errors.Is(err, errs.ErrQueriesPathNotDir) {
		t.Fatalf("expected ErrQueriesPathNotDir, got %v", err)
	}
}

func TestValidatePathsValid(t *testing.T) {
	t.Parallel()
	queriesPath := t.TempDir()
	migrationsPath := t.TempDir()
	if err := conf.ValidatePaths(t.TempDir(), queriesPath, migrationsPath); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidatePathsMigrationsPathInvalid(t *testing.T) {
	t.Parallel()
	err := conf.ValidatePaths("", "", filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, errs.ErrMigrationsPathInvalid) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected invalid migration path with missing-file cause, got %v", err)
	}
}

func TestValidatePathsMigrationsPathNotDir(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "0001_users.sql")
	if err := os.WriteFile(path, []byte("DO 0;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conf.ValidatePaths("", "", path); !errors.Is(err, errs.ErrMigrationsPathNotDir) {
		t.Fatalf("expected ErrMigrationsPathNotDir, got %v", err)
	}
}

func TestValidatePathsOutputPathNotDir(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(path, []byte("preserve this file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conf.ValidatePaths(path, "", ""); !errors.Is(err, errs.ErrOutputPathNotDir) {
		t.Fatalf("expected ErrOutputPathNotDir, got %v", err)
	}
}

func TestValidatePathsOutputPathParentNotDir(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conf.ValidatePaths(filepath.Join(parent, "generated"), "", ""); !errors.Is(err, errs.ErrOutputPathInvalid) && !errors.Is(err, errs.ErrOutputPathNotDir) {
		t.Fatalf("expected invalid output path, got %v", err)
	}
}

func TestValidatePathsMissingOutputPathIsNotCreated(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "generated")
	if err := conf.ValidatePaths(path, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("path validation created output directories: %v", err)
	}
}

func TestValidatePathsOutputSymlinks(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(dir, "valid")
	if err := os.Symlink(t.TempDir(), valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		invalid bool
	}{
		{path: broken, invalid: true},
		{path: filepath.Join(broken, "generated"), invalid: true},
		{path: valid},
		{path: filepath.Join(valid, "generated")},
	} {
		t.Run(tc.path, func(t *testing.T) {
			err := conf.ValidatePaths(tc.path, "", "")
			if tc.invalid {
				if !errors.Is(err, errs.ErrOutputPathInvalid) {
					t.Fatalf("expected ErrOutputPathInvalid, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("valid directory symlink rejected: %v", err)
			}
		})
	}
}

func TestValidatePathsOutputParentTraversal(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{file, broken} {
		t.Run(filepath.Base(parent), func(t *testing.T) {
			// Join would clean away the component this regression must validate.
			path := parent + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "output"
			if err := conf.ValidatePaths(path, "", ""); !errors.Is(err, errs.ErrOutputPathInvalid) {
				t.Fatalf("expected invalid raw output path, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "output")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("validation created output: %v", err)
			}
		})
	}
}

func TestValidatePathsOutputSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	child := filepath.Join(target, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(child, link); err != nil {
		t.Fatal(err)
	}
	// Cleaning link/../output would resolve to this file, but the supplied path
	// actually points to a missing directory beneath target.
	if err := os.WriteFile(filepath.Join(dir, "output"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathSeparator)
	path := link + separator + ".." + separator + "output"
	for _, suffix := range []string{"", separator + "nested", separator + separator} {
		t.Run("suffix="+suffix, func(t *testing.T) {
			if err := conf.ValidatePaths(path+suffix, "", ""); err != nil {
				t.Fatalf("valid symlink-parent output rejected: %v", err)
			}
			if _, err := os.Stat(filepath.Join(target, "output")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("validation created output: %v", err)
			}
		})
	}
}
