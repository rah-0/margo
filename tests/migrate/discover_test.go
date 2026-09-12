package migrate_test

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/migrate"
)

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"0010_Ten.sql", "0002_two-parts.sql", "1_first.sql", "README.md", "notes.txt"} {
		writeMigration(t, dir, name)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.sql"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeMigration(t, filepath.Join(dir, "nested.sql"), "invalid.sql")

	migrations, err := migrate.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []uint64
	for _, migration := range migrations {
		versions = append(versions, migration.Version)
		if filepath.Dir(migration.Path) != dir {
			t.Fatalf("migration path %q is outside %q", migration.Path, dir)
		}
	}
	if !slices.Equal(versions, []uint64{1, 2, 10}) {
		t.Fatalf("versions = %v, want [1 2 10]", versions)
	}
}

func TestDiscoverInvalidFilename(t *testing.T) {
	for _, name := range []string{
		"notes.sql", "0001_.sql", "0000_zero.sql", "-1_negative.sql", "0001_name.up.sql",
		"0001_name.down.sql", "0001_bad name.sql", "0001_é.sql", "0001_name.SQL",
		"18446744073709551616_overflow.sql",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeMigration(t, dir, name)
			_, err := migrate.Discover(dir)
			if !errors.Is(err, errs.ErrInvalidFilename) || !strings.Contains(err.Error(), name) {
				t.Fatalf("discover error = %v, want ErrInvalidFilename naming %q", err, name)
			}
		})
	}
}

func TestDiscoverDuplicateVersion(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "0001_first.sql")
	writeMigration(t, dir, "1_second.sql")
	if _, err := migrate.Discover(dir); !errors.Is(err, errs.ErrDuplicateVersion) {
		t.Fatalf("discover error = %v, want ErrDuplicateVersion", err)
	}
}

func TestDiscoverVersionBounds(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "00000000000000000000000000001_first.sql")
	writeMigration(t, dir, "18446744073709551615_last.sql")
	migrations, err := migrate.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != math.MaxUint64 {
		t.Fatalf("migrations = %v", migrations)
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
