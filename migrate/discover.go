package migrate

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"

	"github.com/rah-0/margo/util"
)

var filenamePattern = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.sql$`)

func discover(path string) ([]Migration, error) {
	paths, err := util.GetSQLFilesInDir(path)
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	versions := make(map[uint64]string, len(paths))
	for _, path := range paths {
		matches := filenamePattern.FindStringSubmatch(filepath.Base(path))
		if matches == nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidFilename, path)
		}
		version, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrInvalidFilename, path, err)
		}
		if version == 0 {
			return nil, fmt.Errorf("%w: %q: version must be positive", ErrInvalidFilename, path)
		}
		if previous, exists := versions[version]; exists {
			return nil, fmt.Errorf("%w: %d in %q and %q", ErrDuplicateVersion, version, previous, path)
		}
		versions[version] = path
		migrations = append(migrations, Migration{Version: version, Path: path})
	}
	slices.SortFunc(migrations, func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})
	return migrations, nil
}

func pendingMigrations(migrations []Migration, current uint64) ([]Migration, error) {
	first := 0
	for first < len(migrations) && migrations[first].Version <= current {
		first++
	}
	pending := migrations[first:]
	for _, migration := range pending {
		// A pending version is greater than current, so current cannot overflow.
		expected := current + 1
		if migration.Version != expected {
			return nil, fmt.Errorf("%w: expected %d before %q (version %d)", ErrVersionGap, expected, migration.Path, migration.Version)
		}
		current = migration.Version
	}
	return pending, nil
}
