package migrate

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rah-0/margo/errs"
)

var filenamePattern = regexp.MustCompile(`^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.sql$`)

// Discover validates SQL migration filenames directly inside path and returns
// their paths and versions sorted by increasing version. It ignores non-SQL
// files and subdirectories, rejects zero or duplicate versions, and permits
// numbering gaps. PendingMigrations validates the pending sequence against a
// database's current version.
func Discover(path string) ([]Migration, error) {
	// Preserve the empty disk path's read error: DirFS("") reports an invalid
	// filesystem instead of the missing directory reported by os.ReadDir("").
	if path == "" {
		_, err := os.ReadDir(path)
		return nil, fmt.Errorf("read SQL directory %q: %w", path, err)
	}
	migrations, err := discover(os.DirFS(path), path)
	if err != nil {
		return nil, err
	}
	for i := range migrations {
		migrations[i].Path = filepath.Join(path, migrations[i].Path)
	}
	return migrations, nil
}

// DiscoverFS validates SQL migration filenames directly inside source's root
// directory (".") and returns relative names sorted by increasing version.
// It follows Discover's filename rules and leaves sequence validation to
// PendingMigrations. A nil source returns an error matching fs.ErrInvalid.
// The caller retains ownership of source; discovery only reads its directory.
func DiscoverFS(source fs.FS) ([]Migration, error) {
	return discover(source, "")
}

// discover keeps lookup names relative. directory supplies disk context for
// diagnostics only and must never be used to clean or rebuild the source root.
func discover(source fs.FS, directory string) ([]Migration, error) {
	root := "."
	if directory != "" {
		root = directory
	}
	if source == nil {
		return nil, &fs.PathError{Op: "readdir", Path: root, Err: fs.ErrInvalid}
	}
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("read SQL directory %q: %w", root, err)
	}

	var migrations []Migration
	versions := make(map[uint64]string, len(entries))
	for _, entry := range entries {
		filename := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(filename), ".sql") {
			continue
		}
		path := filename
		if directory != "" {
			path = filepath.Join(directory, filename)
		}
		matches := filenamePattern.FindStringSubmatch(filename)
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
		migrations = append(migrations, Migration{Version: version, Path: filename})
	}
	slices.SortFunc(migrations, func(a, b Migration) int {
		return cmp.Compare(a.Version, b.Version)
	})
	return migrations, nil
}

// PendingMigrations returns the consecutive migrations newer than current, or
// errs.ErrVersionGap when that pending sequence skips a version. It expects migrations
// sorted by increasing version, as returned by Discover or DiscoverFS. The returned suffix
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
