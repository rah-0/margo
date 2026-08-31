# Margo Migration Package — Plan

A self-contained `migrate` package living next to `db/`, `template/`, `util/`, and `conf/`. It does **not** depend on the code generator and can be imported by any Go service that already has a `*sql.DB` against MariaDB/MySQL.

## 1. Goals

- Discover, validate, lock, execute, and track SQL migrations in a single, explicit pipeline.
- Two files per version: `{N}_{Name}.up.sql` (required) and `{N}_{Name}.down.sql` (optional). No in-file split markers.
- One process at a time via `GET_LOCK` — the table is for observability, not concurrency.
- Detect drift by hashing every migration file and comparing on each run.
- Fail loudly. Never silently repair, skip, or rewrite history.

## 2. Package Layout

Mirrors the existing one-concern-per-file convention used in `conf/`, `db/`, `template/`, `util/`:

```
migrate/
  type.go            // status constants, defaults
  structs.go         // Options, Migration, MigrationStatus
  errors.go          // typed sentinel errors
  parse.go           // filename regex + parsing
  discover.go        // directory scan + pairing + validation + sort
  hash.go            // BOM trim, CRLF->LF, sha256
  schema.go          // CREATE TABLE IF NOT EXISTS for the metadata table
  lock.go            // GET_LOCK / RELEASE_LOCK on a dedicated *sql.Conn
  store.go           // CRUD on the metadata table
  runner.go          // Migrate / Up / Down / Validate / Status orchestration
  baseline.go        // Baseline (adoption: stamp rows applied without executing SQL)
  parse_test.go
  discover_test.go
  hash_test.go
integration/
  migrate_test.go    // tagged Testcontainers coverage for store/runner/baseline
```

Errors are wrapped with `nabu.FromError(err).WithArgs(...).Log()`, identical to the rest of the codebase.

## 3. Public API

```go
type Options struct {
    DB              *sql.DB
    Path            string
    LockName        string        // default: "margo_schema_migrations"
    LockTimeout     time.Duration // default: 30s
    TableName       string        // default: "schema_migrations"
    Strict          bool          // default: true (missing applied file => fail)
    UseTransactions bool          // default: true
}

type State string

const (
    StateRunning    State = "running"
    StateApplied    State = "applied"
    StateFailed     State = "failed"
    StateRolledBack State = "rolled_back"
)

type Migration struct {
    Version  uint64 // numeric prefix, leading zeros stripped
    Name     string // as on disk (case preserved)
    UpPath   string
    DownPath string // "" when no down file
    UpHash   string // sha256 hex of normalized up content
    DownHash string // "" when no down file
}

type MigrationStatus struct {
    Version     uint64
    Name        string
    UpHash      string
    DownHash    sql.NullString
    State       State
    StartedAt   time.Time
    FinishedAt  sql.NullTime
    ExecutionMs sql.NullInt64
    ErrorText   sql.NullString
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// Migrate brings the database to the requested target version.
//
//   target == nil      catch up to the highest filesystem version (default).
//   *target == 0       roll back every applied migration.
//   *target > current  apply up   migrations in (current, *target] ascending.
//   *target < current  apply down migrations in (*target, current] descending.
//   *target == current no-op (after pre-flight checks pass).
//
// *target must be 0 or equal to an existing filesystem version, else ErrTargetNotFound.
func Migrate(ctx context.Context, opts Options, target *uint64) error

// Up is sugar for Migrate(ctx, opts, nil).
func Up(ctx context.Context, opts Options) error

// Down is sugar for Migrate(ctx, opts, &zero) — rolls back every applied migration.
func Down(ctx context.Context, opts Options) error

// Baseline marks filesystem migrations up to target as applied without
// executing their SQL. Used to adopt margo on a database whose schema was
// provisioned outside the migration system. See §11.1.
//
//   target == nil  stamp every filesystem migration as applied.
//   *target == 0   no-op.
//   *target > 0    stamp filesystem migrations with version <= *target.
//
// Idempotent: rows already present (any status) are not touched; only
// missing rows are inserted with status='applied'.
func Baseline(ctx context.Context, opts Options, target *uint64) error

func Validate(ctx context.Context, opts Options) error
func Status(ctx context.Context, opts Options) ([]MigrationStatus, error)
```

`Migrate` is the only execution primitive — `Up` and `Down` are one-line wrappers so callers don't have to build a `*uint64` for the common cases. Direction is **derived** from comparing the resolved target to the current applied version (see §9.1). The caller never tells the package which way to walk.

`Baseline` is the **non-executing** primitive: it writes metadata rows for migrations that describe schema already present in the database. It is the answer to "we already have a production database; how do we start using margo without breaking it?"

`Validate` runs every check (filesystem pairing, hash drift, dirty state, linear-history gap detection) **without** acquiring the advisory lock or executing any migration — useful for CI smoke tests.

### 3.1 Prior Art

The unified `Migrate(target)` shape mirrors `golang-migrate`'s `(*Migrate).Migrate(version uint)`. `goose` reaches the same behaviour via two named verbs (`UpTo`, `DownTo`); the underlying state machine is identical. We pick the unified form because it collapses "where am I going" and "how do I get there" into one decision point and makes the no-target / target-equals-current edge cases trivially uniform.

`Baseline` mirrors Flyway's `flyway baseline` and Liquibase's `changelog-sync`. Both tools converged on this shape because it's the only safe answer for adopting a migration tool on an existing database — automatic schema-effect detection (read: schema diffing, see §16) is unreliable in the general case and would require introspection logic that contradicts margo's "plain SQL only, no ORM" stance.

## 4. Migration File Format

Single regex used by `parse.go`:

```
^([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.(up|down)\.sql$
```

- Group 1 — `version`: any number of digits, leading zeros allowed; parsed with `strconv.ParseUint`.
- Group 2 — `name`: must start with `[A-Za-z0-9]`; allows `_` and `-`. Stored as-is, compared **case-insensitively** when pairing up/down.
- Group 3 — `direction`: `up` or `down`.

Anything else (e.g. `README.md`, `.keep`, `001_foo.sql`, `001_foo.up.sql.bak`) is silently ignored at discovery — never an error.

## 5. Discovery & Validation

`discover.go` — pure, no DB:

1. `os.ReadDir(opts.Path)` (non-recursive).
2. For each entry: try the regex; ignore on miss.
3. Group matches by `version`:
   - More than one `up` for the same version → `ErrDuplicateVersion`.
   - More than one `down` for the same version → `ErrDuplicateVersion`.
   - Up and down with names that differ on case-insensitive comparison → `ErrNameMismatch`.
4. A version with only a `down` file → `ErrOrphanDown`.
5. Sort by `version` ascending (numeric, not lexical).
6. For each accepted pair, compute hashes and build a `Migration`.

`Validate(ctx, opts)` additionally cross-checks the metadata table (see §9, steps 3–6) without acquiring the lock.

## 6. Hashing (`hash.go`)

```
func hashFile(path string) (string, error)
```

1. Read full file (`os.ReadFile`).
2. Trim leading UTF-8 BOM `EF BB BF` if present.
3. Replace every `\r\n` with `\n` (single pass).
4. SHA-256 over the resulting bytes; return lowercase hex (64 chars → fits `CHAR(64)`).

Comments are **never** stripped — a whitespace or comment change is a real change and must invalidate the hash.


## 7. Metadata Table (`schema.go`)

Created idempotently on every entry to `Up` / `Down` / `Validate` / `Status`. Table name comes from `opts.TableName` (default `schema_migrations`).

```sql
CREATE TABLE IF NOT EXISTS `schema_migrations` (
  `version`      BIGINT UNSIGNED NOT NULL,
  `name`         VARCHAR(255)    NOT NULL,
  `up_hash`      CHAR(64)        NOT NULL,
  `down_hash`    CHAR(64)        NULL,
  `status`       VARCHAR(32)     NOT NULL,
  `started_at`   DATETIME(6)     NOT NULL,
  `finished_at`  DATETIME(6)     NULL,
  `execution_ms` BIGINT          NULL,
  `error_text`   TEXT            NULL,
  `created_at`   DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at`   DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`version`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

The table is intentionally narrow and append-only at the row level: rows are **never deleted**, only transitioned. The status column captures the full lifecycle and supports later additions like `rolled_back`.

## 8. Concurrency — Advisory Lock (`lock.go`)

The advisory lock is the only real concurrency primitive. The table is observability.

- Pull a dedicated connection: `conn, _ := opts.DB.Conn(ctx)`. Hold it for the duration of the run.
- `SELECT GET_LOCK(?, ?)` with `(opts.LockName, int(opts.LockTimeout.Seconds()))`.
  - Returns `1` → acquired.
  - Returns `0` → timeout → `ErrLockTimeout`.
  - Returns `NULL` → driver/server error → `ErrLockFailed`.
- Defer `SELECT RELEASE_LOCK(?)` and `conn.Close()`. Connection close is the safety net — if the process dies, MariaDB drops the lock automatically.
- All metadata-table writes during the run go through the **same** `*sql.Conn`. Migration execution (DDL/DML) also goes through this connection, so the lock is held continuously.
- Lock name default: `"margo_schema_migrations"`. Configurable for cases where a single DB hosts multiple independent applications.

## 9. Execution Flow — `Migrate`

The runner is one pipeline shared by `Migrate`, `Up`, `Down`, and (in read-only mode) `Validate` / `Status`. The order below is **the order**: each step depends only on the steps above it, never below.

1. **Defaults + arg validation.** Apply `applyDefaults(*Options)`. Reject `DB == nil` (programmer error → panic) and `Path == ""` (`ErrPathRequired`).
2. **Acquire connection + advisory lock.** `conn, _ := opts.DB.Conn(ctx)`; `GET_LOCK(opts.LockName, opts.LockTimeout)`. Defer release. (Skipped by `Validate` and `Status`.)
3. **Ensure metadata table.** `CREATE TABLE IF NOT EXISTS …` on `conn`.
4. **Discover filesystem migrations (§5).** Pure, no DB. Result: `[]Migration` sorted ascending by version, hashes computed.
5. **Load DB state.** `SELECT … FROM <TableName>` → `map[uint64]MigrationStatus`.
6. **Pre-flight (read-only cross-check).** Runs against the in-memory filesystem list and DB map; never executes migration SQL:
   - Status `running` or `failed` for any row → `ErrDirtyState`. Operator must clear manually.
   - For each row with status `applied` or `rolled_back`:
     - File missing on disk → `ErrMissingFile` if `Strict`; otherwise `nabu`-log and continue.
     - `up_hash` differs from disk → `ErrHashMismatch`.
     - Disk has a down file and stored `down_hash` differs → `ErrHashMismatch`.
     - Stored `down_hash IS NULL` but disk now has a down file → `ErrHashMismatch` (history rewritten).
   - **Linear-history check.** Let `current` = max version in DB with `status='applied'`. Any filesystem migration with `version < current` that is **not** present in the DB map → `ErrNonLinearHistory`. (This is the "branch-merge gap" case; out-of-order application is deferred to a future flag, see §15.)
7. **Resolve target.** This is the only step that consumes the caller's `target` argument:
   - `target == nil` → `resolved = max(filesystem.versions)`; `0` when no files exist.
   - `*target == 0` → `resolved = 0` (full rollback).
   - `*target > 0` → `resolved = *target`. Must exist in the filesystem list, else `ErrTargetNotFound`.
8. **Compute current.** `current = max(version | row.State == StateApplied)`; `0` when no applied rows.
9. **Plan.** Pure function over the discovered list, the DB map, `current`, and `resolved`:
   - `resolved == current` → empty plan, no-op return.
   - `resolved > current` → **forward plan**: filesystem migrations with `current < version <= resolved` in ascending order. Each must currently be in pending state (no DB row, or row status `rolled_back`). The linear-history check above guarantees no applied row sits in this window.
   - `resolved < current` → **backward plan**: DB rows with `resolved < version <= current` and `status='applied'`, in descending order. For each, the filesystem must still have a matching down file (`DownPath != ""`) and the disk down-hash must equal the stored `down_hash`, else `ErrNoDownFile` / `ErrHashMismatch`.
10. **Execute the plan in order.** Per step in §9.2 below. Stop on the first error and surface it (the lock is released by the deferred path, but no further migrations are attempted).
11. **Release lock + close connection.**

### 9.1 Direction Decision (Worked Examples)

```
filesystem: 1, 2, 3, 4, 5
db applied: 1, 2

Migrate(nil)        → resolved=5, current=2 → forward plan: [3, 4, 5]
Migrate(&3)         → resolved=3, current=2 → forward plan: [3]
Migrate(&2)         → resolved=2, current=2 → no-op
Migrate(&1)         → resolved=1, current=2 → backward plan: [2.down]
Migrate(&0)         → resolved=0, current=2 → backward plan: [2.down, 1.down]
Migrate(&7)         → ErrTargetNotFound (7 not on filesystem)
Up()                ≡ Migrate(nil)
Down()              ≡ Migrate(&zero)
```

Edge case — gap: `filesystem: 1, 2, 3`, `db applied: 1, 3`. Pre-flight raises `ErrNonLinearHistory` (version 2 is on disk but not applied while 3 is applied). The user must either apply 2 manually with the deferred out-of-order flag, or renumber.

### 9.2 Per-Migration Execution

For each entry in the resolved plan, on the locked `*sql.Conn`:

1. Upsert the row to running state: `(version, name, up_hash, down_hash, status='running', started_at=NOW(6), finished_at=NULL, execution_ms=NULL, error_text=NULL)`. For backward steps the row already exists; the upsert clears the timing/error fields.
2. If `UseTransactions`: `tx, _ := conn.BeginTx(ctx, nil)`.
3. Execute the chosen SQL — `up.sql` for forward steps, `down.sql` for backward steps — on `tx` (or `conn` if no tx).
4. On error:
   - `tx.Rollback()` (best effort; DDL may have already committed implicitly).
   - Update row: `status='failed'`, `finished_at=NOW(6)`, `execution_ms`, `error_text=err.Error()`.
   - Return `ErrMigrationFailed` wrapping the original error plus the version. Stop the plan.
5. On success:
   - `tx.Commit()` (no-op when no tx).
   - Update row:
     - Forward step → `status='applied'`.
     - Backward step → `status='rolled_back'` (row preserved for audit; a later forward step will flip it back to `applied`).
   - Set `finished_at=NOW(6)`, `execution_ms`, `error_text=NULL`.

### 9.3 SQL Statement Execution

The driver does not allow multiple statements in a single `Exec` unless the DSN sets `multiStatements=true`. Two strategies:

- **v1 (recommended):** require `multiStatements=true` on the caller's `*sql.DB`. Document this in the package doc and in the README. `Exec` the entire file in one call. Simple, no SQL parsing.
- **Fallback (deferred):** a tiny statement splitter that splits on `;` outside of `'…'`, `"…"`, `` `…` ``, line comments (`-- …`, `# …`), and block comments (`/* … */`). The existing `template.StripSQLComments` already handles those quoting cases and can be lifted into a shared helper.

The plan ships v1 only. If a caller cannot enable `multiStatements`, they can pre-split and run statements externally; this is documented as a limitation, not a silent failure.

## 10. Transaction Policy

- `UseTransactions=true` (default): wrap each migration's up SQL in a `BeginTx` / `Commit`.
- MariaDB/MySQL DDL (`CREATE`, `ALTER`, `DROP`, `TRUNCATE`, …) causes an **implicit commit**. The package documents this clearly: a DDL failure mid-migration cannot be rolled back, and the row will be marked `failed` with the partial state recorded in `error_text`.
- `UseTransactions=false`: every statement runs directly on the locked connection. Use this for migrations that mix DDL + DML and want to avoid spurious "transaction was committed" surprises.
- Per-migration transaction overrides are deferred (see §15). The global flag is the only knob in v1.

## 11. `Up` and `Down` Wrappers

Both are one-liners over `Migrate`:

```go
func Up(ctx context.Context, opts Options) error {
    return Migrate(ctx, opts, nil)
}

func Down(ctx context.Context, opts Options) error {
    zero := uint64(0)
    return Migrate(ctx, opts, &zero)
}
```

`Up` is the daily-driver call: catch up to the latest filesystem version. `Down` is the nuclear option: roll back every applied migration. There is intentionally **no** "roll back N steps" primitive in v1 — operators who need that walk the `Status` output and call `Migrate(ctx, opts, &targetVersion)` with the explicit version they want to land on. This keeps every operation expressible in one mental model: "where do you want the schema to be?"

A `rolled_back` row is treated as pending by the next forward call, so `Down` followed by `Up` is a clean re-apply (subject to the up-hash still matching disk).

### 11.1 Adoption — `Baseline`

Margo must be safe to drop into a project that already has a populated database. Two situations come up:

**Case A — naturally idempotent migrations.** The migration files are written defensively (`CREATE TABLE IF NOT EXISTS …`, `INSERT IGNORE …`, etc.) and the existing schema already matches what they would produce. In this case `Up` simply works: the SQL runs as a no-op against an already-correct schema, the row lands in `status='applied'`, and the up-hash records the file content. No special handling needed.

**Case B — non-idempotent migrations against an existing schema.** The migration SQL would error or duplicate state if executed against the current database — e.g. `ALTER TABLE users ADD COLUMN email …` against a table that already has `email`, or `INSERT INTO roles …` against a table that already has those rows. Running `Up` here would fail and the row would land in `status='failed'`.

`Baseline` is the answer to Case B. It writes metadata rows for every filesystem migration up to `target` with `status='applied'`, current timestamps, and hashes computed from disk — but **does not execute any SQL**. After baselining, `Up` only runs migrations strictly newer than the baseline.

Algorithm (mirrors §9 up to step 6, then diverges):

1. Defaults + arg validation.
2. Acquire connection + advisory lock.
3. Ensure metadata table.
4. Discover filesystem migrations (§5).
5. Load DB state.
6. Reduced pre-flight: hash-drift, missing-file, and non-linear-history checks all run; the dirty-state check still runs (a baseline cannot fix a `running`/`failed` row).
7. Resolve target (same rules as `Migrate` — §9 step 7).
8. For each filesystem migration with `version <= resolved`:
   - If a row already exists in the DB map → skip (any status; baseline does not overwrite history).
   - Otherwise insert: `(version, name, up_hash, down_hash, status='applied', started_at=NOW(6), finished_at=NOW(6), execution_ms=0, error_text=NULL)`.
9. Release lock + close connection.

Adoption workflow, end to end:

1. Operator writes migration files matching the **current** state of the production database (versions `001`..`N`).
2. Operator runs `Baseline(ctx, opts, nil)` once. All `N` rows land in the metadata table as `applied`, no SQL is executed.
3. From this point forward, normal `Up` / `Migrate` runs treat versions `> N` as pending and execute them.
4. If a developer later finds the baseline files don't actually match the schema, they correct the files — but the corrected hash will then trigger `ErrHashMismatch` on the next run, surfacing the discrepancy explicitly. Operator must either (a) re-baseline (after manually clearing the affected rows) or (b) accept that the file genuinely is out of sync and address it in a new migration.

`Baseline` is **idempotent**: running it twice produces the same metadata table state. It is **not** retroactive: it never converts a `failed` row to `applied`, and it never deletes rows.

What `Baseline` deliberately does **not** do:
- It does not introspect the live schema. It trusts the operator to have authored migration files that describe the current state.
- It does not auto-detect "this `CREATE TABLE IF NOT EXISTS` would have been a no-op anyway, so just stamp it" — that path is already handled by Case A above and needs no special code.
- It does not bypass the dirty-state pre-flight; if the table has a `running` or `failed` row, `Baseline` returns `ErrDirtyState`, identical to `Migrate`. Adoption assumes a clean slate in the metadata table (typically empty).

## 12. Errors (`errors.go`)

Sentinel errors so callers and tests can use `errors.Is`. All paths through the runner wrap these with `nabu.FromError(err).WithArgs(version, name, ...).Log()` for context.

```go
var (
    ErrPathRequired      = errors.New("migrate: opts.Path is required")
    ErrInvalidFilename   = errors.New("migrate: filename does not match {version}_{name}.{up|down}.sql")
    ErrDuplicateVersion  = errors.New("migrate: duplicate migration version")
    ErrOrphanDown        = errors.New("migrate: down file without matching up file")
    ErrNameMismatch      = errors.New("migrate: up/down filename names differ for the same version")
    ErrLockTimeout       = errors.New("migrate: could not acquire advisory lock within timeout")
    ErrLockFailed        = errors.New("migrate: GET_LOCK returned NULL")
    ErrMissingFile       = errors.New("migrate: previously applied migration file missing on disk")
    ErrHashMismatch      = errors.New("migrate: applied migration hash does not match disk")
    ErrDirtyState        = errors.New("migrate: previous migration left the database in a dirty state")
    ErrMigrationFailed   = errors.New("migrate: migration execution failed")
    ErrNoDownFile        = errors.New("migrate: rollback requested but down file is missing")
    ErrTargetNotFound    = errors.New("migrate: target version not found on filesystem")
    ErrNonLinearHistory  = errors.New("migrate: filesystem migration sits below the current applied version but is not applied")
)
```

`ErrMigrationFailed` always wraps the underlying driver error so callers can drill in with `errors.Unwrap` or `errors.As`. `ErrTargetNotFound` and `ErrNonLinearHistory` are the two new sentinels introduced by target-aware execution (§9).

## 13. Configuration & Defaults

Applied by an internal `applyDefaults(*Options)` at the top of every public function:

| Field             | Default                          | Notes                                              |
|-------------------|----------------------------------|----------------------------------------------------|
| `DB`              | (required)                       | `nil` → programmer error, panic.                   |
| `Path`            | (required)                       | Empty → `ErrPathRequired`.                         |
| `LockName`        | `"margo_schema_migrations"`      | Override per-application when sharing a database. |
| `LockTimeout`     | `30 * time.Second`               | Passed as integer seconds to `GET_LOCK`.           |
| `TableName`       | `"schema_migrations"`            | Validated against `^[A-Za-z_][A-Za-z0-9_]*$` to avoid SQL injection in DDL/DML strings; identifiers are never user data. |
| `Strict`          | `true`                           | Missing applied file fails instead of warns.       |
| `UseTransactions` | `true`                           | Best-effort given DDL semantics.                   |

## 14. Test Plan

### Unit (no DB)

- `parse_test.go`
  - Valid filenames with various zero-padding (`1_x.up.sql`, `01_x.up.sql`, `0001_x.up.sql`).
  - Mixed-case names accepted (`001_AddProducts.up.sql`).
  - Reject filenames not starting with digits + `_` (`abc_x.up.sql`, `_001_x.up.sql`).
  - Reject malformed extensions (`001_x.sql`, `001_x.up.sql.bak`, `001_x.UP.SQL`).
  - Reject empty name (`001_.up.sql`).
- `discover_test.go` (uses `t.TempDir()`)
  - Happy path: pairs detected, sorted by numeric version (`2_x.up.sql` after `10_x.up.sql` is wrong → must be `2` before `10`).
  - Down without up → `ErrOrphanDown`.
  - Two `up` files for the same version → `ErrDuplicateVersion`.
  - Two `down` files for the same version → `ErrDuplicateVersion`.
  - Up without down → accepted, `DownPath == ""`.
  - Unrelated files (`README.md`, `.keep`, `.DS_Store`, `notes.sql`) ignored.
  - Up/down with different names for same version → `ErrNameMismatch`.
  - Case-insensitive name pairing (`001_Foo.up.sql` + `001_foo.down.sql`) is accepted.
- `hash_test.go`
  - `\r\n` and `\n` versions of the same content hash identically.
  - Leading BOM ignored.
  - Trailing whitespace and comments are part of the hash (changing them changes the hash).
  - Identical files → identical hashes.

### Integration (requires Docker; gated by the `integration` build tag)

Migration integration tests should use the repository's disposable MariaDB
harness and run with the rest of the `integration` package. No external test
checkout, `go.work` file, fixed port, or manually managed database is required.
Each test uses `t.TempDir()` for migration files and a unique `TableName` to
avoid cross-test pollution.

- Metadata table is created on first run; second run is a no-op.
- `Up` applies all migrations and rows reflect `status='applied'` with monotonic versions.
- Editing an applied `.up.sql` after the fact triggers `ErrHashMismatch`.
- Adding a `.down.sql` to an already-applied migration triggers `ErrHashMismatch`.
- A migration whose SQL fails leaves `status='failed'`, populates `error_text`; subsequent `Up` returns `ErrDirtyState`.
- Removing a previously applied file with `Strict=true` → `ErrMissingFile`; with `Strict=false` → logged, run continues.
- Concurrency: launch two `Up` calls in parallel goroutines on the same DB; the second returns `ErrLockTimeout` once `LockTimeout` elapses. The first completes normally.
- **Target-aware execution** (covers §9 explicitly):
  - `Migrate(ctx, opts, &v)` with `v` equal to current applied version → no-op, no rows touched.
  - `Migrate(ctx, opts, &v)` with `v > current` → forward plan applies migrations in `(current, v]`.
  - `Migrate(ctx, opts, &v)` with `v < current` and all required down files present → backward plan rolls back `(v, current]` in descending order; rows become `rolled_back`.
  - `Down()` (== `Migrate` to 0) on a fully-applied 3-migration set → all three rows go `rolled_back`; subsequent `Up()` re-applies all three.
  - `Migrate(ctx, opts, &v)` with `v` not present on filesystem → `ErrTargetNotFound`, no DB writes.
  - Backward target across a version whose down file is missing → `ErrNoDownFile`, no partial rollback.
  - Filesystem with versions `[1, 2, 3]`, DB applied `[1, 3]` → any call (including `Validate`) returns `ErrNonLinearHistory`.
- **Adoption / `Baseline`** (covers §11.1):
  - Empty metadata table + 3 filesystem migrations + `Baseline(nil)` → 3 rows land as `applied`, no SQL executed (proven by inserting a sentinel SELECT-NOW marker that would fail if the up SQL had run).
  - `Baseline(&2)` on the same setup → only versions 1 and 2 get rows; version 3 stays pending and a subsequent `Up` runs only it.
  - Re-running `Baseline` is a no-op (idempotent — row count unchanged, hashes unchanged).
  - Pre-existing `applied` row + `Baseline(nil)` → existing row untouched (hash, timestamp, status preserved); only missing rows added.
  - `Baseline` against a metadata table with a `failed` row → `ErrDirtyState`.
- `Validate` returns the same errors as `Migrate`'s pre-flight without acquiring the lock or executing anything.

## 15. Forward-Compat & Open Questions

- **Per-migration transaction override.** Add a column `use_transaction TINYINT(1) NULL` and/or a filename suffix (e.g. `001_foo.notx.up.sql`). Deferred. The hash field is content-only, so a directive on the filename does not affect hashes.
- **Embedded migrations** (`embed.FS`). Add an `FS fs.FS` field on `Options` later; `Path` becomes a sub-path within the FS. Discovery code should be written against an interface that both `os` and `fs.FS` satisfy to make this drop-in.
- **Statement splitter.** Ship as `migrate.SplitStatements([]byte) [][]byte` when needed. The hash function stays content-only and is unaffected.
- **Out-of-order application** (`AllowOutOfOrder bool` on `Options`). Today `ErrNonLinearHistory` is raised when a pending migration sits below the current applied version. A future flag would suppress it and treat such migrations as pending. The plan, the storage schema, and the runner are already shaped for this — only the pre-flight check needs to be made conditional. Deferred so v1 has one and only one execution model.
- **CLI command.** A `margo migrate up|down|to|status|validate` subcommand is straightforward once the package is stable, but is out of scope for v1 to keep the existing `main.go` flag surface unchanged.
- **Multiple migration directories.** A single `Path` keeps the API trivially safe; revisit only if a concrete caller demands it.

## 16. Explicit Non-Goals

The following will **not** be added — to v1 or later. They are listed here so future contributors don't re-litigate the design.

- **Squashing or merging migrations.** Replacing N applied files with a single consolidated `001_initial_schema.up.sql` and deleting the originals (Django's `manage.py squashmigrations`). Every existing applied row would point to a file that no longer exists (→ `ErrMissingFile` / `ErrHashMismatch`), forcing every deployed environment to re-baseline. The history is meant to be append-only and auditable; collapsing it is an explicit non-goal. Operators who truly need a fresh starting point write the consolidated file by hand, clear the relevant rows manually, and run `Baseline` (§11.1) — outside the package's responsibility.
- **Automated repair of dirty state.** A SQL transaction does **not** solve this on MariaDB: every DDL statement (`CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `RENAME`, …) issues an *implicit commit* (§10), so a multi-statement up file that fails on statement 3 has already committed statements 1 and 2 — `ROLLBACK` is a no-op against them. The runner already mitigates the **observability** side of this by writing a `running` row before execution and transitioning it to `applied` / `failed` afterwards (§9.2), so a crashed process leaves a visible breadcrumb. What it does **not** do, and will not, is decide on the operator's behalf how to recover: running the down file (can compound damage when the down SQL references the half-built state), retrying the up file (can silently re-corrupt the schema), or clearing the row and continuing (hides a real failure) are all unsafe in the general case. The operator inspects the schema, fixes the SQL, manually adjusts the affected row (`UPDATE schema_migrations SET status='rolled_back' WHERE version=?` or `DELETE`), and re-runs. This is the documented escape hatch.
- **Steps-based relative move** (`Steps(±n)` like golang-migrate). Trivially expressible as `Migrate(ctx, opts, &(current ± n))` from the caller side; adding it to the API would introduce a second mental model alongside the absolute-target one without buying anything.
- **Schema diffing or auto-generated migrations.** Tools like Atlas or Prisma introspect the live schema, compare it to a desired schema, and emit migration SQL automatically. Margo treats migration files as the authoritative source of truth — the operator writes them. Diffing requires deep schema introspection that contradicts the package's "plain SQL only, no ORM" stance and would make the executed DDL non-auditable.
- **Repair / force-reset commands.** No `margo migrate repair` or `--force` flag. Failed rows are cleared manually with SQL; see the dirty-state item above.

## 17. Implementation Order

1. `type.go`, `structs.go`, `errors.go` — declarations only.
2. `parse.go` + `parse_test.go`.
3. `hash.go` + `hash_test.go`.
4. `discover.go` + `discover_test.go` (uses `t.TempDir()`).
5. `schema.go` + `lock.go` + `store.go` + `store_test.go` (integration).
6. `runner.go` — `Migrate` + `Up` / `Down` wrappers + `Validate` + `Status` + `runner_test.go`. Forward and backward planning land in the same commit since they share one execution loop.
7. `baseline.go` — `Baseline` + `baseline_test.go`. Reuses the lock, schema, store, and discovery code from steps 4–5; only the per-row insertion differs from the runner.
8. README addendum documenting `multiStatements=true`, the unified `Migrate(target)` model, the adoption / `Baseline` workflow, and a one-screen usage example.

Each step lands as its own commit to keep the diff reviewable and the test surface obvious.
