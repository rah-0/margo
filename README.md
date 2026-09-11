<p align="center">
  <img src="margo.png" alt="MarGO: MariaDB's sea lion with Go's gopher" width="280">
</p>

# MarGO

Generate Go database code from MariaDB schemas, with optional SQL migrations.

MarGO reads your tables and generates structs, CRUD methods, and functions for
custom SQL. The generated code uses explicit field mapping, `database/sql`,
cached prepared statements, contexts, and transactions. It has no MarGO runtime
dependency.

**All generated column fields are strings.** SQL `NULL` becomes `""` on read,
so generated values do not distinguish NULL from an empty string.

Requires **Go 1.27 or newer**. Tested with **MariaDB 12.3.3**; MySQL compatibility,
including migration session checks, has not been verified.

[Quick start](#quick-start) · [Migrations](#migrations) ·
[Generated code](#using-generated-code) · [Custom SQL](#custom-sql) ·
[CLI reference](#cli-reference) · [Development](#development)

## Quick start

Install the CLI:

```bash
go install github.com/rah-0/margo@latest
```

From your application's Go module, generate code for an existing database.
This example uses a database named `app`; set `DB_PASSWORD` to its user's password:

```bash
margo \
  -dbUser=app \
  -dbPassword="$DB_PASSWORD" \
  -dbName=app \
  -dbIp=127.0.0.1 \
  -outputPath=./generated
```

MarGO validates output paths before database work, rejecting existing files and
broken symbolic links. Symlinks are followed, so the real output destination must
be inside a Go module for generated imports to resolve correctly. For a new
project, initialize one with `go mod init example.com/app`.

Two optional flags extend this command:

- `-migrationsPath=./migrations` creates the database if missing and applies SQL
  migrations **before any schema inspection or code generation**.
- `-queriesPath=./queries` adds functions for your custom SQL queries.

Without `-migrationsPath`, MarGO uses the existing database and skips migrations.
If a migration fails, generation stops immediately: existing generated files
stay untouched, and missing output directories are not created.

`-outputPath` is optional: omit it to run migrations without generating code.
Custom queries require an output directory, so `-queriesPath` must be paired
with `-outputPath`. If none of the three paths is set, MarGO prints a warning
and exits successfully without requiring credentials or connecting to a database.

For an `app` database containing a `users` table, the output is:

```text
generated/
└── App/
    ├── queries.go        # SetDB, transaction helpers, standalone queries
    └── Users/
        └── entity.go     # Entity, field constants, CRUD, table-mapped queries
```

Each application table gets its own package. `queries.go` is always generated,
even without custom queries. Database, table, and column names become Go
identifiers: `app` → `App`, `user_profiles` → `UserProfiles`,
`last_update` → `LastUpdate`. Hyphens, dots, and camel-case boundaries are also
handled. Regeneration overwrites the generated files; keep application code
separate.

## Migrations

Keep migrations in a separate directory, with one forward-only SQL file per
version. Use four-digit zero-padding:

```text
migrations/
├── 0001_users.sql
└── 0002_email.sql
```

`0001_users.sql`:

```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);
```

`0002_email.sql`:

```sql
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email VARCHAR(255) NOT NULL DEFAULT '';
```

Add `-migrationsPath=./migrations` to the [quick-start command](#quick-start).
The database user needs permission to create the configured database and run
the migration SQL. Entities and custom-query bindings then use the updated schema.

To apply migrations alone, omit `-outputPath` and `-queriesPath`:

```bash
margo \
  -dbUser=app \
  -dbPassword="$DB_PASSWORD" \
  -dbName=app \
  -dbIp=127.0.0.1 \
  -migrationsPath=./migrations
```

This mode does not inspect tables or generate files and needs no Go module in
the working directory.

### Versions and repeated runs

MarGO keeps the last completed version in a single row of
`margo_schema_version`. This reserved table is always excluded from generation,
including runs without migrations enabled.

- A fresh database starts at version `0`; its first migration is `0001`.
- Pending versions must be consecutive. At version `4`, files `0005` and `0007`
  are rejected because `0006` is missing, before either pending file runs.
- Completed versions are skipped. Editing an applied file does not rerun it;
  add a new migration for subsequent changes.
- Applied files need not remain on disk, but fresh databases still need the
  complete sequence from `0001`.

Filenames follow `{version}_{name}.sql`. Versions are positive `uint64` numbers;
leading zeros do not change their value. Names start with an ASCII letter or
digit and contain only ASCII letters, digits, underscores, or hyphens. Duplicate
versions and malformed SQL filenames are rejected. Subdirectories and non-SQL
files are ignored.

### Execution and recovery

Each file runs unchanged as one SQL execution. There are no up/down sections,
statement splitting, rollback commands, or migration history records. The CLI
enables multi-statement execution automatically. An advisory lock serializes
migrators for the same database, with a 30-second wait limit and context cancellation.

The stored version advances only after a file succeeds. On SQL or version-update
failure, MarGO stops and reports the file, version, and failed operation.

**Write SQL that is safe to retry, including data changes.** MariaDB DDL can
commit implicitly, so a failed file may leave earlier changes applied. A file
can also run again if its SQL succeeded but its version update failed. Inspect
the database, make the failed migration safe to retry if necessary, and rerun
the command. MarGO resumes from the stored version without automatic repair.

`CREATE TABLE IF NOT EXISTS` only guards creation; it does not update an existing
table's definition. Use a new migration for later schema changes.

Before initializing version storage and after each file, MarGO requires the
original database to be selected, autocommit to be enabled, and no transaction
to remain open. Complete `START TRANSACTION` … `COMMIT` blocks within a file are
allowed. A violation returns `migrate.ErrInvalidSessionState`, leaves that file's
version unrecorded, and stops generation.

The dedicated connection is physically closed after lock cleanup on every run.
This isolates session settings and rolls back any uncommitted transactional work;
previously committed changes remain. MarGO never commits an unfinished transaction.

### Use the migrator from Go

The migrator can also run independently of code generation:

```go
import "github.com/rah-0/margo/migrate"

err := migrate.Run(ctx, migrate.Options{
    DB:   database,
    Path: "./migrations",
})
```

Supply a `*sql.DB` with the target database selected, `multiStatements=true`,
and autocommit enabled. The package does not create databases and leaves the
caller's pool open. Errors support `errors.Is` and `errors.As`; for example,
`migrate.ErrVersionGap` identifies a missing pending version.

## Using generated code

Open a `database/sql` pool with a registered MySQL driver, then call `SetDB` on
the generated database package once during initialization. It shares the pool
with every generated table package.

For module `example.com/app` and the output above:

```go
import (
    "context"
    "database/sql"
    "fmt"

    appdb "example.com/app/generated/App"
    users "example.com/app/generated/App/Users"
)

func listUsers(ctx context.Context, database *sql.DB) error {
    if err := appdb.SetDB(database); err != nil {
        return err
    }

    result := users.DBSelectAllCtx(ctx)
    if result.Error != nil {
        return result.Error
    }
    for _, user := range result.Entities {
        fmt.Println(user.Id, user.Name)
    }
    return nil
}
```

### CRUD and field selection

Each table package exposes `Entity`, `FQTN` (the qualified SQL table name),
`Field<Name>` constants, and `Fields` in schema order. Use those constants with
`NewQueryParams()` to select columns and filters:

```go
user := &users.Entity{Name: "Ada"}
result := user.DBSelectCtx(ctx, users.NewQueryParams().
    WithSelect(users.FieldId, users.FieldName).
    WithWhere(users.FieldName))
```

Filters compare the selected fields to values on the receiver, joined with `AND`.
Use `WithInsert` to omit columns that should receive database defaults.

| Operation | Behavior |
| --- | --- |
| `entity.DBInsert(params)` | Inserts columns from `WithInsert`; defaults to all columns. |
| `entity.DBUpdate(params)` | Requires both `WithUpdate` and `WithWhere`. |
| `entity.DBDelete(params)` | Deletes rows matching `WithWhere`; defaults to matching all columns. |
| `entity.DBSelect(params)` | Uses `WithSelect` and `WithWhere`; defaults to all columns and no filter. |
| `entity.DBExists(params)` | Requires non-nil params. Selects and filters on all columns unless specified; on a match, fills the receiver and sets `Exists`. |
| `users.DBSelectAll()` | Reads every row and column. |
| `users.DBTruncate()` | Truncates the table. |

Every operation returns a result with an `Error` field; check it before using
the other fields. Reads populate `Entities`, writes populate `Result`
(`sql.Result`), and `DBExists` populates `Exists` while updating the receiver.
Single-row custom queries use `Entity` and `Exists`.

### Contexts and transactions

CRUD operations and custom queries have `Ctx`, `Tx`, and `CtxTx` variants:

```go
user.DBInsert(params)
user.DBInsertCtx(ctx, params)
user.DBInsertTx(tx, params)
user.DBInsertCtxTx(ctx, tx, params)
```

The database package provides `NewTx()`, `NewCtxTx(ctx)`, `NewTxOpts(opts)`, and
`NewCtxTxOpts(ctx, opts)`, each returning `(*sql.Tx, error)`. Commit or roll back
the transaction yourself:

```go
tx, err := appdb.NewCtxTx(ctx)
if err != nil {
    return err
}
defer tx.Rollback()

user := &users.Entity{Id: "1", Name: "Ada"}
result := user.DBInsertCtxTx(ctx, tx, users.NewQueryParams().
    WithInsert(users.FieldId, users.FieldName))
if result.Error != nil {
    return result.Error
}
return tx.Commit()
```

Prepared statements are cached by query string and reused across transactions.

## Custom SQL

Pass `-queriesPath=./queries` and put one query in each `.sql` file directly
inside that directory. Use UpperCamelCase filenames: the filename becomes the
function name, prefixed with `Query` or `Exec`. List columns explicitly;
`SELECT *` is rejected.

For example, `queries/GetUserById.sql`:

```sql
-- Returns: id name email
-- ResultMode: one
-- MapAs: users
SELECT id, name, email
FROM users
WHERE id = ?;
```

`MapAs: users` places the function in the `Users` package and reuses its `Entity`:

```go
result := users.QueryGetUserByIdCtx(ctx,
    users.NewQueryParams().WithParams("1"))
if result.Error != nil {
    return result.Error
}
if result.Exists {
    fmt.Println(result.Entity.Name)
}
```

### SQL tags

Put each tag on its own line. Separate field names with whitespace, **not commas**.

| Tag | Meaning |
| --- | --- |
| `-- Returns: id name` | Required for row results. Lists fields in the exact order of the SQL result columns. |
| `-- ResultMode: many` | `many` (default), `one`, or `exec`; determines how results are read. |
| `-- MapAs: users` | Uses an existing table's entity and package. The table name must match the schema exactly. |
| `-- Params: id` | Optional documentation only; it does not control generated parameters. |

With `MapAs`, each `Returns` name must be an exact column name from the mapped
table. Without it, MarGO generates a standalone function in `App/queries.go`
with its own `Query<Name>Result` and `Query<Name>ResultInner` row type for row
results. Result field names are normalized to Go identifiers, and their values
are strings.

| Mode | Function | Result fields |
| --- | --- | --- |
| `many` | `Query<Name>` | `Entities`, `Error` |
| `one` | `Query<Name>` | `Entity`, `Exists`, `Error`; no rows means `Entity=nil`, `Exists=false`. |
| `exec` | `Exec<Name>` | `Result` (`sql.Result`), `Error`; no `Returns` tag needed. |

Design `one` queries to return at most one row: mapped queries reject multiple
rows, while standalone queries read the first row.

SQL containing `?` generates a `*QueryParams` argument. Pass a non-nil
`NewQueryParams().WithParams(...)`, with values in placeholder order. Queries
without `?` have no params argument. Custom functions use the same `Ctx`, `Tx`,
and `CtxTx` suffixes as CRUD methods. Ordinary SQL comments are removed when
generating custom queries.

See the [SQL fixtures](integration/testdata/queries) and
[generated-code usage tests](integration/testdata/generated_runtime_test.go)
for more examples.

## CLI reference

| Flag | Required | Default | Purpose |
| --- | --- | --- | --- |
| `-dbUser` | Yes | — | Database username. |
| `-dbPassword` | Yes | — | Non-empty database password. |
| `-dbName` | Yes | — | Database to inspect and migrate. |
| `-dbIp` | Yes | — | Database hostname or IP address. |
| `-dbPort` | No | `3306` | Database port. |
| `-outputPath` | No | — | Enables code generation into this directory inside a Go module; created if missing. |
| `-queriesPath` | No | — | Existing directory of custom SQL queries; requires `-outputPath`. |
| `-migrationsPath` | No | — | Existing directory of numbered SQL migrations. |

Database credentials are required when generation or migrations are requested.
The CLI writes completion, warning, and error logs to stderr and exits non-zero
on failure. With no paths selected, it warns and exits successfully without work.

## Development

Unit tests need no database or Docker:

```bash
go test -count=1 -race -cover -covermode=atomic ./...
```

With Docker available, run the MariaDB integration suite:

```bash
go test -tags=integration -count=1 -race -cover -covermode=atomic ./integration
```

The suite starts disposable MariaDB containers with random ports and cleans them
up afterward. It covers migrations and runs generated CRUD and custom-query code
in temporary Go modules. No local database credentials or checked-in generated
output are needed.

For performance comparisons with raw SQL, Bun, Ent, and GORM, see
[BENCHMARKS.md](BENCHMARKS.md) for results, methodology, and commands.

---

[Support MarGO](https://www.buymeacoffee.com/rah.0)
