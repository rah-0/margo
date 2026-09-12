package migrate

import (
	"database/sql"
	"io/fs"
)

// Options configures a migration run. Supply exactly one of Path or FS.
type Options struct {
	DB   *sql.DB // Caller-owned connection pool with a selected database and multiStatements enabled.
	Path string  // Directory containing numbered SQL files.
	// FS contains numbered SQL files directly in its root directory (".").
	// Use fs.Sub to select a subdirectory. The caller keeps FS valid and stable
	// throughout Run; MarGO does not write to it or take ownership of it.
	FS fs.FS
}

// Migration identifies a numbered SQL file.
type Migration struct {
	Version uint64
	// Path is a disk path from Discover, or a relative filename suitable for
	// fs.ReadFile from DiscoverFS.
	Path string
}
