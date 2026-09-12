package migrate_test

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
)

func TestDiscover(t *testing.T) {
	files := fstest.MapFS{
		"0010_Ten.sql":           {Data: []byte("DO 0;")},
		"0002_two-parts.sql":     {Data: []byte("DO 0;")},
		"1_first.sql":            {Data: []byte("DO 0;")},
		"README.md":              {},
		"notes.txt":              {},
		"nested.sql/invalid.sql": {},
	}
	forDiscoverySources(t, files, func(t *testing.T, discover func() ([]migrate.Migration, error), path func(string) string) {
		migrations, err := discover()
		if err != nil {
			t.Fatal(err)
		}
		want := []migrate.Migration{
			{Version: 1, Path: path("1_first.sql")},
			{Version: 2, Path: path("0002_two-parts.sql")},
			{Version: 10, Path: path("0010_Ten.sql")},
		}
		if !slices.Equal(migrations, want) {
			t.Fatalf("migrations = %v, want %v", migrations, want)
		}
		if _, err := migrate.PendingMigrations(migrations, 0); !errors.Is(err, errs.ErrVersionGap) {
			t.Fatalf("pending error = %v, want ErrVersionGap after successful discovery", err)
		}
	})
}

func TestDiscoverInvalidFilename(t *testing.T) {
	for _, name := range []string{
		"notes.sql", "0001_.sql", "0000_zero.sql", "-1_negative.sql", "0001_name.up.sql",
		"0001_name.down.sql", "0001_bad name.sql", "0001_é.sql", "0001_name.SQL",
		"18446744073709551616_overflow.sql",
	} {
		t.Run(name, func(t *testing.T) {
			forDiscoverySources(t, fstest.MapFS{name: {}}, func(t *testing.T, discover func() ([]migrate.Migration, error), path func(string) string) {
				_, err := discover()
				if !errors.Is(err, errs.ErrInvalidFilename) || !strings.Contains(err.Error(), path(name)) {
					t.Fatalf("discover error = %v, want ErrInvalidFilename naming %q", err, path(name))
				}
			})
		})
	}
}

func TestDiscoverDuplicateVersion(t *testing.T) {
	files := fstest.MapFS{"0001_first.sql": {}, "1_second.sql": {}}
	forDiscoverySources(t, files, func(t *testing.T, discover func() ([]migrate.Migration, error), path func(string) string) {
		_, err := discover()
		if !errors.Is(err, errs.ErrDuplicateVersion) || !strings.Contains(err.Error(), path("0001_first.sql")) || !strings.Contains(err.Error(), path("1_second.sql")) {
			t.Fatalf("discover error = %v, want ErrDuplicateVersion naming both migrations", err)
		}
	})
}

func TestDiscoverVersionBounds(t *testing.T) {
	files := fstest.MapFS{"00000000000000000000000000001_first.sql": {}, "18446744073709551615_last.sql": {}}
	forDiscoverySources(t, files, func(t *testing.T, discover func() ([]migrate.Migration, error), _ func(string) string) {
		migrations, err := discover()
		if err != nil {
			t.Fatal(err)
		}
		if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != math.MaxUint64 {
			t.Fatalf("migrations = %v", migrations)
		}
	})
}

func TestDiscoverEmptyRoot(t *testing.T) {
	forDiscoverySources(t, fstest.MapFS{}, func(t *testing.T, discover func() ([]migrate.Migration, error), _ func(string) string) {
		migrations, err := discover()
		if err != nil || len(migrations) != 0 {
			t.Fatalf("empty root migrations = %v, error = %v", migrations, err)
		}
	})
}

// The disk fixture is independent of embedded fixtures used by integration tests.
func forDiscoverySources(t *testing.T, files fstest.MapFS, test func(*testing.T, func() ([]migrate.Migration, error), func(string) string)) {
	t.Helper()
	t.Run("disk", func(t *testing.T) {
		dir := t.TempDir()
		for name, file := range files {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, file.Data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		test(t, func() ([]migrate.Migration, error) { return migrate.Discover(dir) }, func(name string) string { return filepath.Join(dir, name) })
	})
	t.Run("filesystem", func(t *testing.T) {
		test(t, func() ([]migrate.Migration, error) { return migrate.DiscoverFS(files) }, func(name string) string { return name })
	})
}

func TestDiscoverInvalidDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", filepath.Join(dir, "missing"), file} {
		t.Run(path, func(t *testing.T) {
			_, err := migrate.Discover(path)
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) || !strings.Contains(err.Error(), path) {
				t.Fatalf("Discover(%q) error = %v, want inspectable filesystem error with disk context", path, err)
			}
			if path != file && !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Discover(%q) error = %v, want fs.ErrNotExist", path, err)
			}
		})
	}
}

func TestDiscoverPreservesSymlinkRoot(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(target, "child"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeMigration(t, target, "0001_target.sql")
	writeMigration(t, dir, "invalid.sql")
	root := link + string(filepath.Separator) + ".."
	migrations, err := migrate.Discover(root)
	want := []migrate.Migration{{Version: 1, Path: filepath.Join(root, "0001_target.sql")}}
	if err != nil || !slices.Equal(migrations, want) {
		t.Fatalf("Discover(%q) = %v, %v; want %v", root, migrations, err, want)
	}
}

func TestPendingMigrations(t *testing.T) {
	tests := []struct {
		name     string
		current  uint64
		versions []uint64
		want     []uint64
		missing  uint64
	}{
		{name: "fresh", versions: []uint64{1, 2, 3}, want: []uint64{1, 2, 3}},
		{name: "newer", current: 2, versions: []uint64{1, 2, 3, 4}, want: []uint64{3, 4}},
		{name: "historical files removed", current: 10, versions: []uint64{11, 12}, want: []uint64{11, 12}},
		{name: "missing first", versions: []uint64{2, 3}, missing: 1},
		{name: "missing intermediate", versions: []uint64{1, 2, 4}, missing: 3},
		{name: "missing next", current: 4, versions: []uint64{6}, missing: 5},
		{name: "validate all pending", current: 4, versions: []uint64{5, 7}, missing: 6},
		{name: "no files"},
		{name: "all completed", current: 3, versions: []uint64{1, 2, 3}},
		{name: "history incomplete", current: 3, versions: []uint64{1, 3}},
		{name: "maximum reached", current: math.MaxUint64, versions: []uint64{math.MaxUint64}},
		{name: "last possible version", current: math.MaxUint64 - 1, versions: []uint64{math.MaxUint64}, want: []uint64{math.MaxUint64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var migrations []migrate.Migration
			for _, version := range tt.versions {
				migrations = append(migrations, migrate.Migration{Version: version, Path: fmt.Sprintf("%04d_change.sql", version)})
			}
			pending, err := migrate.PendingMigrations(migrations, tt.current)
			if tt.missing != 0 {
				if !errors.Is(err, errs.ErrVersionGap) || !strings.Contains(err.Error(), fmt.Sprintf("expected %d ", tt.missing)) {
					t.Fatalf("pending error = %v, want missing version %d", err, tt.missing)
				}
				if len(pending) != 0 {
					t.Fatalf("gap returned executable migrations: %v", pending)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []uint64
			for _, migration := range pending {
				got = append(got, migration.Version)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("pending versions = %v, want %v", got, tt.want)
			}
		})
	}
}

func writeMigration(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("DO 0;"), 0o600); err != nil {
		t.Fatal(err)
	}
}
