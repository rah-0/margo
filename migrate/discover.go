package migrate

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/util"
)

var filenamePattern = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.sql$`)

// Discover validates SQL migration filenames directly inside path and returns
// their paths and versions sorted by increasing version. It ignores non-SQL
// files and subdirectories, rejects zero or duplicate versions, and permits
// numbering gaps. PendingMigrations validates the pending sequence against a
// database's current version.
func Discover(path string) ([]Migration, error) {
	paths, err := util.GetSQLFilesInDir(path)
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	versions := make(map[uint64]string, len(paths))
	for _, path := range paths {
		matches := filenamePattern.FindStringSubmatch(filepath.Base(path))
		if matches == nil {
			return nil, fmt.Errorf("%w: %q", errs.ErrInvalidFilename, path)
		}
		version, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", errs.ErrInvalidFilename, path, err)
		}
		if version == 0 {
			return nil, fmt.Errorf("%w: %q: version must be positive", errs.ErrInvalidFilename, path)
		}
		if previous, exists := versions[version]; exists {
			return nil, fmt.Errorf("%w: %d in %q and %q", errs.ErrDuplicateVersion, version, previous, path)
		}
		versions[version] = path
		migrations = append(migrations, Migration{Version: version, Path: path})
	}
	slices.SortFunc(migrations, func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})
	return migrations, nil
}

// PendingMigrations returns the consecutive migrations newer than current, or
// errs.ErrVersionGap when that pending sequence skips a version. It expects migrations
// sorted by increasing version, as returned by Discover. The returned suffix
// shares the input slice's backing array; this function does not modify it.
func PendingMigrations(migrations []Migration, current uint64) ([]Migration, error) {
	first := 0
	for first < len(migrations) && migrations[first].Version <= current {
		first++
	}
	pending := migrations[first:]
	for _, migration := range pending {
		// A pending version is greater than current, so current cannot overflow.
		expected := current + 1
		if migration.Version != expected {
			return nil, fmt.Errorf("%w: expected %d before %q (version %d)", errs.ErrVersionGap, expected, migration.Path, migration.Version)
		}
		current = migration.Version
	}
	return pending, nil
}
