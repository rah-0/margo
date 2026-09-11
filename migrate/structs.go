package migrate

import "database/sql"

// Options configures a migration run.
type Options struct {
	DB   *sql.DB // Caller-owned connection pool with a selected database and multiStatements enabled.
	Path string  // Directory containing numbered SQL files.
}

// Migration identifies a numbered SQL file.
type Migration struct {
	Version uint64
	Path    string
}
