package runner

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strings"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/structs"
)

// Options selects migrations, generation, and connection ownership for one run.
type Options struct {
	// DB is owned by the caller and remains open on every return path. Every
	// connection must consistently select the same existing database. Migrations
	// require multiStatements, autocommit, and no open transaction; see migrate.Run.
	DB *sql.DB
	// Connection selects pools owned by MarGO. Only migrations create a missing
	// database. Supply exactly one of DB or Connection for an active operation.
	Connection *structs.ConnectionOptions

	// OutputPath enables generation and must have an enclosing go.mod. Missing
	// directories are created only after migrations succeed.
	OutputPath string
	// Inputs selects the read-only filesystems used by this run.
	Inputs Inputs
}

// Inputs contains SQL sources rooted at their migration or query directory.
// Use os.DirFS for disk directories or fs.Sub to select a filesystem subdirectory.
// The caller keeps sources valid and stable throughout Run and retains ownership.
// MarGO reads direct children of "." and closes the file handles it opens.
type Inputs struct {
	// Migrations enables numbered SQL migrations before generation. A non-nil
	// filesystem enables migrations even when its root directory is empty.
	Migrations fs.FS
	// Queries contains custom and named SQL files. A non-nil filesystem requires
	// OutputPath even when its root directory is empty.
	Queries fs.FS
}

type connectionField struct {
	name  string
	value string
}

type filesystemInput struct {
	name   string
	source fs.FS
}

func (opts Options) validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Inputs.Queries != nil && opts.OutputPath == "" {
		return errs.ErrQueriesWithoutOutput
	}
	if err := conf.ValidateOutputPath(opts.OutputPath); err != nil {
		return err
	}
	for _, input := range []filesystemInput{
		{"queries", opts.Inputs.Queries},
		{"migrations", opts.Inputs.Migrations},
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if input.source != nil {
			if _, err := fs.ReadDir(input.source, "."); err != nil {
				return fmt.Errorf("read %s filesystem root: %w", input.name, err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.DB != nil && opts.Connection != nil {
		return errs.ErrConnectionConflict
	}
	if opts.DB == nil && opts.Connection == nil {
		return errs.ErrConnectionRequired
	}
	if opts.Connection != nil {
		var missing []string
		for _, field := range []connectionField{
			{"User", opts.Connection.User},
			{"Password", opts.Connection.Password},
			{"Host", opts.Connection.Host},
			{"Database", opts.Connection.Database},
		} {
			if field.value == "" {
				missing = append(missing, field.name)
			}
		}
		if len(missing) != 0 {
			return fmt.Errorf("%w: missing %s", errs.ErrInvalidConnection, strings.Join(missing, ", "))
		}
	}
	return nil
}
