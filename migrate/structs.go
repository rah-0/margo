package migrate

import (
	"database/sql"
	"io/fs"
)

// Options configures a migration run.
type Options struct {
	DB *sql.DB // Caller-owned connection pool with a selected database and multiStatements enabled.
	// FS is required and contains numbered SQL files directly in its root (".").
	// Use os.DirFS for a disk directory or fs.Sub to select a subdirectory. The
	// caller keeps FS valid and stable throughout Run; MarGO does not write to it
	// or take ownership of it.
	FS fs.FS
}

// Migration identifies a numbered SQL file.
type Migration struct {
	Version uint64
	// Path is a relative filename suitable for fs.ReadFile.
	Path string
}
