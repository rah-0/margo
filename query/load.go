// Package query loads and parses custom SQL queries and their metadata.
package query

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/rah-0/margo/structs"
)

// Load reads SQL files directly from source's root (".") in filename order.
// It ignores subdirectories and non-SQL files, accepts any case for the .sql
// extension, and returns parsed general and table-mapped queries together.
// Use os.DirFS for a disk directory or fs.Sub for a filesystem subdirectory.
// A nil source returns an error matching fs.ErrInvalid.
// The caller retains ownership and keeps source valid and stable during Load.
// Cancellation is checked between operations; it cannot interrupt a blocking
// filesystem operation. No partial results are returned on error.
func Load(ctx context.Context, source fs.FS) ([]structs.NamedQuery, error) {
	if source == nil {
		return nil, &fs.PathError{Op: "readdir", Path: ".", Err: fs.ErrInvalid}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("read queries directory %q: %w", ".", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var queries []structs.NamedQuery
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		filename := entry.Name()
		if entry.IsDir() || !strings.EqualFold(path.Ext(filename), ".sql") {
			continue
		}
		data, err := fs.ReadFile(source, filename)
		if err != nil {
			return nil, fmt.Errorf("read query file %q: %w", filename, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		content := string(data)
		if err := CheckNoSelectStar([]string{content}); err != nil {
			return nil, fmt.Errorf("validate query file %q: %w", filename, err)
		}
		name := strings.TrimSuffix(filename, path.Ext(filename))
		queries = append(queries, ExtractNamedQuery(content, name))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return queries, nil
}
