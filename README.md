![MarGO logo](https://github.com/rah-0/margo-test/blob/master/margo.png "MariaDB's Sea Lion with Golang's Gopher")

[![Go Report Card](https://goreportcard.com/badge/github.com/rah-0/margo?v=1)](https://goreportcard.com/report/github.com/rah-0/margo)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

<a href="https://www.buymeacoffee.com/rah.0" target="_blank">
  <img src="https://cdn.buymeacoffee.com/buttons/v2/arial-orange.png" alt="Buy Me A Coffee" height="50" style="height:50px;">
</a>

# MarGO: A Simple, Reflection-free ORM for MariaDB and Go

MarGO (MariaDB + GO) is a code generator that creates type-safe database interaction code, mapping MariaDB table schemas to Go structs with zero reflection. This approach provides near-raw SQL performance while maintaining some ORM convenience.

> **Compatibility Note**: MarGO has been tested with MariaDB but not MySQL. Since both use the same underlying driver (`github.com/go-sql-driver/mysql`), it should be compatible with MySQL as well, but this has not been explicitly tested.

## Features

- **Reflection-free Design**: MarGO doesn't use reflection at runtime, eliminating that performance cost
- **Code Generation**: Automatically generates Go code from your database schema
- **Prepared Statement Caching**: Improves performance by reusing prepared statements
- **Direct Table Mapping**: Maps database tables directly to Go structs without complex abstractions
- **Context Support**: All database operations have context-aware versions
- **Strongly Typed**: Generated code is type-safe
- **Near-Raw SQL Performance**: Benchmarks show performance within 99-105% of raw SQL
- **Minimal Dependencies**: Lightweight with few external dependencies
- **No Nil Pointer Errors**: Safely handles NULL database fields by sanitizing them to empty strings, preventing the common nil pointer errors that could occur

## How It Works

MarGO works in a single direction, from **Database** to **Code**:

1. It connects to your MariaDB database and reads the schema information
2. For each table, it generates a Go file with:
   - A struct representing the table
   - Constants for field names
   - Type-safe CRUD and some pre-defined general functions
   - Prepared statement caching
3. The generated code uses standard `database/sql` operations

## Output Layout

Given `-outputPath=/path/to/out` and `-dbName=mydb`, MarGO produces:

```
out/
└── Mydb/                # normalized DB name (one Go package)
    ├── queries.go       # SetDB, transaction helpers, standalone custom queries
    ├── Alpha/           # one sub-package per table
    │   └── entity.go    # Entity struct, constants, CRUD, table-bound queries
    └── Beta/
        └── entity.go
```

`queries.go` is always generated, even when `-queriesPath` is not provided, because it hosts `SetDB` and the transaction helpers for the database package.

### Name Normalization

DB, table, and column names are normalized into Go identifiers via a single rule (`db.NormalizeString`):

- `snake_case`, `kebab-case`, and `dotted.names` are split on `_`/`-`/`.` and PascalCased — `last_update` → `LastUpdate`, `test_field` → `TestField`.
- Otherwise, `camelCase` / `PascalCase` input is split on case boundaries and PascalCased — `bigNumber` → `BigNumber`.

The same rule is applied to column names in `-- Returns:` tags, so the SQL can use the literal column names from the schema.

## Getting Started

### Installation

Install the MarGO CLI tool:

```bash
go install github.com/rah-0/margo@latest
```

### CLI Usage

MarGO provides a command-line interface to generate code from your database schema:

```bash
margo -dbUser="your_db_user" \
      -dbPassword="your_db_password" \
      -dbName="your_database" \
      -dbIp="localhost" \
      -dbPort="3306" \
      -outputPath="/path/must/be/directory" \
      -queriesPath="/path/must/be/directory"
```

### CLI Parameters

| Parameter     | Description                                        | Default | Required |
|---------------|----------------------------------------------------|---------|----------|
| `-dbUser`     | Database username                                  | -       | Yes      |
| `-dbPassword` | Database password                                  | -       | Yes      |
| `-dbName`     | Database name                                      | -       | Yes      |
| `-dbIp`       | Database IP address                                | -       | Yes      |
| `-dbPort`     | Database port                                      | 3306    | Yes      |
| `-outputPath` | Directory where generated files will be saved      | -       | Yes      |
| `-queriesPath`| Optional path to directory containing .sql files   | -       | No       |

## Custom SQL Queries

MarGO can turn SQL queries into type-safe Go functions:

### Requirements

- Each custom query must be in its own `.sql` file within the directory specified by `-queriesPath`
- **File names must be in UpperCamelCase** (e.g., `GetUserById.sql`, `DeleteOldRecords.sql`) as they become the function names
- The filename (without `.sql` extension) becomes the generated function name:
  - `many`/`one` modes → `Query<Name>(...)` (e.g., `GetUserById.sql` → `QueryGetUserById`)
  - `exec` mode → `Exec<Name>(...)` (e.g., `DeleteOldRecords.sql` → `ExecDeleteOldRecords`)
- No `SELECT *` queries are allowed, you must explicitly specify columns
- See [example queries directory](https://github.com/rah-0/margo/tree/master/doc/sql/queries) for reference

## Supported Tags (SQL Generator)

### Params
- **Syntax:** `-- Params: uuid_user id ...`
- **Optional — purely documentation.** The tag is parsed but is **not consumed by the generator**: removing it from a `.sql` file produces the same generated code. Whether a function accepts a `*QueryParams` argument is decided solely by whether the SQL contains `?` placeholders.
- Tokens are split by **whitespace only** (any run of spaces/tabs); commas and other punctuation are *not* separators and will become part of the token.
- The whole tag must be on a single line.
- Placeholders remain positional `?` and are bound in call order via `QueryParams.WithParams(...)`.

### Returns
- **Syntax:** `-- Returns: field_a field_b field_c`
- **Required** for `many` or `one` modes
- Tokens are split by **whitespace only** (any run of spaces/tabs) — do **not** comma-separate. For example `-- Returns: field_a, field_b` would be parsed as `["field_a,", "field_b"]` and break code generation.
- The whole tag must be on a single line.
- Order defines the struct field order
- Field names are normalized with `db.NormalizeString`
- All fields are string (`NULL → ""`).

### ResultMode
- **Syntax:** `-- ResultMode: many | one | exec`
- **Optional**, defaults to `many`

All three modes return `*Query<Name>Result` (or `*QueryResult` when `-- MapAs:` is set), differing only in which fields are populated:

| Mode   | Function name      | Populated fields on the result struct       |
|--------|--------------------|---------------------------------------------|
| many   | `Query<Name>`      | `Entities`, `Error`                         |
| one    | `Query<Name>`      | `Entity`, `Exists`, `Error` (no rows → `Exists=false`, `Entity=nil`) |
| exec   | `Exec<Name>`       | `Result`, `Error`                           |

Each generated function also has `Ctx`, `Tx`, and `CtxTx` variants — for example `QueryGetUserByIdCtx(ctx, params)`, `QueryGetUserByIdTx(tx, params)`, `QueryGetUserByIdCtxTx(ctx, tx, params)`. The `params *QueryParams` argument is only emitted when the SQL contains `?` placeholders.

### MapAs
- **Syntax:** `-- MapAs: table_name`
- **Optional but highly significant**
- Defines the table entity this query belongs to
- When set, the generated function is placed inside that table’s `entity.go` file and reuses the entity’s `*QueryResult` — `Entities` is `[]*Entity` and `Entity` is `*Entity` from that table
- When omitted, the query is treated as a **standalone custom query**, generating its own `Query<Name>Result` (with a `Query<Name>ResultInner` row type) and appearing in the database package’s `queries.go`
- In short:
  **With MapAs → operates on an existing entity.**
  **Without MapAs → creates a new custom result type.**

### Field Mapping Rules (with MapAs)

- Use the table’s **exact column names** in both `SELECT` and `-- Returns:` (e.g., `uuid`, `last_update`, `test_field`).
- Backticks not required; aliases/expressions not supported with MapAs.
- Order doesn’t matter; names must exist in the table.
- The generator normalizes to Go fields via `db.NormalizeString`  
  (`uuid`→`Uuid`, `last_update`→`LastUpdate`, `test_field`→`TestField`).

**Example** (file: `GetRecentCats.sql`)
```sql
-- Returns: uuid last_update
-- ResultMode: many
-- MapAs: alpha
SELECT uuid, last_update
FROM alpha
WHERE animal = 'cat' AND last_update > '2024-01-01 00:00:00.000000'
```

The filename `GetRecentCats.sql` becomes the function name `QueryGetRecentCats()`.

### Notes
- Comments starting with `-- ...` or `# ...` are ignored.
- Block comments `/* ... */` are stripped safely (handles quotes).

### Using Generated Query Functions

Before using any generated code (tables or custom queries), you must set the database connection once on the database package. `SetDB` cascades the connection to every generated table sub-package, so a single call wires up the whole schema:

```go
import (
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
    "yourProject/yourDatabase"
)

func main() {
    conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/yourdb")
    if err != nil {
        // Handle error
    }

    if err := yourDatabase.SetDB(conn); err != nil {
        // Handle error
    }

    // Standalone custom query (no MapAs) — lives on the DB package
    res := yourDatabase.QueryGetUserById(
        yourDatabase.NewQueryParams().WithParams("admin"),
    )
    if res.Error != nil { /* ... */ }
    _ = res.Entity // *QueryGetUserByIdResultInner for ResultMode: one
}
```

## Generated API

### Per-Entity Constants and Variables

For each table, `entity.go` exposes:

- `FQTN` — the fully qualified `` `db`.`table` `` identifier used in generated SQL.
- `Field<Name>` — one untyped string constant per column (e.g., `FieldUuid`, `FieldLastUpdate`).
- `Fields` — a `[]string` listing every `Field<Name>` in declaration order.

The struct itself maps every column to a `string`, with `NULL` values normalized to `""` on read:

```go
type Entity struct {
    Uuid       string `json:",omitempty,omitzero"`
    LastUpdate string `json:",omitempty,omitzero"`
    Animal     string `json:",omitempty,omitzero"`
    // ...
}
```

### CRUD Methods

Each `*Entity` exposes the following methods, every one of which has `Ctx`, `Tx`, and `CtxTx` variants:

| Method       | Purpose                                                              |
|--------------|----------------------------------------------------------------------|
| `DBInsert`   | Insert one row. `params.Insert` selects columns; defaults to all.    |
| `DBUpdate`   | `UPDATE … SET … WHERE …` — requires both `params.Update` and `params.Where`. |
| `DBDelete`   | `DELETE … WHERE …` (AND-combined). `params.Where` selects columns; defaults to all. |
| `DBSelect`   | `SELECT … FROM … [WHERE …]`. `params.Select` and `params.Where` are optional. |
| `DBExists`   | `… LIMIT 1` lookup; on hit, copies the row into the receiver and sets `Exists=true`. |

Package-level helpers (also with `Ctx`/`Tx`/`CtxTx` variants):

- `DBSelectAll()` — select every row with every column.
- `DBTruncate()` — `TRUNCATE TABLE`.
- `SetDB(*sql.DB) error` — wires the connection (called for you by the parent DB package).

### `QueryParams`

`QueryParams` is a fluent builder that controls which fields participate in each operation:

```go
qp := Alpha.NewQueryParams().
    WithSelect(Alpha.FieldUuid, Alpha.FieldAnimal).
    WithWhere(Alpha.FieldAnimal)

e := &Alpha.Entity{Animal: "cat"}
res := e.DBSelect(qp)
```

Available builder methods: `WithSelect`, `WithWhere`, `WithInsert`, `WithUpdate`, and `WithParams` (positional `?` arguments for custom queries).

### `QueryResult`

Every CRUD and custom-query call returns `*QueryResult` (or `*Query<Name>Result` for standalone queries):

```go
type QueryResult struct {
    Entities []*Entity   // DBSelect / DBSelectAll / Query<Name> (many)
    Entity   *Entity     // DBExists hit / Query<Name> (one)
    Error    error
    Result   sql.Result  // DBInsert / DBUpdate / DBDelete / DBTruncate / Exec<Name>
    Exists   bool        // DBExists / Query<Name> (one)
}
```

### Transactions

The DB package generates four constructors:

```go
NewTx() (*sql.Tx, error)
NewCtxTx(ctx context.Context) (*sql.Tx, error)
NewTxOpts(opts *sql.TxOptions) (*sql.Tx, error)
NewCtxTxOpts(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
```

Pass the returned `*sql.Tx` to any `*Tx` / `*CtxTx` method:

```go
tx, err := yourDatabase.NewTx()
if err != nil { /* ... */ }

e := &Alpha.Entity{Uuid: u, Animal: "cat"}
if r := e.DBInsertTx(tx, nil); r.Error != nil {
    _ = tx.Rollback()
    return r.Error
}
return tx.Commit()
```

Prepared statements are cached per query string and reused across transactions via `Tx.Stmt` / `Tx.StmtContext`.

## Generated Code

You can see examples of generated code in the [margo-test repository](https://github.com/rah-0/margo-test/tree/master/dbs/Template).

For examples of how to use the generated code, see these test files:
- [Entity usage examples](https://github.com/rah-0/margo-test/blob/master/dbs/Template/Alpha/entity_test.go)
- [Custom queries usage examples](https://github.com/rah-0/margo-test/blob/master/dbs/Template/queries_test.go)

## Performance

See the detailed benchmark results in the [BENCHMARKS.md](https://github.com/rah-0/margo-test/blob/master/BENCHMARKS.md) file in the margo-test repository.

---

[![Buy Me A Coffee](https://cdn.buymeacoffee.com/buttons/default-orange.png)](https://www.buymeacoffee.com/rah.0)
