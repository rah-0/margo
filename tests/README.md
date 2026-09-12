# Testing

Run the commands below from the repository root. Tests live in a separate Go
module: `tests/go.mod` uses a local replacement for `github.com/rah-0/margo` that
points to the parent checkout, so no `go.work` is required. Running
`go test ./...` at the repository root does not include this nested module.

The coverage commands explicitly include `github.com/rah-0/margo/...` to measure
production packages.

## Unit tests

Unit tests need no database or Docker. Filesystem cases exercise disk sources,
in-memory sources, and the standard helpers' fallback through `Open`:

```bash
GOWORK=off go -C tests test -count=1 -race -cover -covermode=atomic \
  -coverpkg=github.com/rah-0/margo/... ./...
```

## MariaDB integration

With Docker available, run the integration suite:

```bash
GOWORK=off go -C tests test -tags=integration -count=1 -race -cover -covermode=atomic \
  -coverpkg=github.com/rah-0/margo/... ./integration
```

The suite starts disposable MariaDB containers on random ports and cleans them
up afterward. It checks disk and directly embedded migrations through both Go
APIs, database bootstrap, migration failures and reruns, and equivalent generated
output. Disk sources use `os.DirFS`; embedded sources are passed directly through
`runner.Inputs` and `migrate.Options.FS`. Custom-query inputs cover general and
table-mapped queries, including generation after embedded migrations. Generation
and generated CRUD and named queries use temporary Go modules; migration-only
runs need no Go module. No existing database or local database credentials are
required.

## Static checks

Check the production module and ordinary and tagged test packages:

```bash
GOWORK=off go vet ./...
GOWORK=off go -C tests vet ./...
GOWORK=off go -C tests vet -tags=integration ./integration
GOWORK=off go -C tests vet -tags=benchmark ./benchmark
```

## Benchmarks

Compile the optional benchmark harness without running its database benchmarks:

```bash
GOWORK=off go -C tests test -tags=benchmark -run '^$' -count=1 -race -cover -covermode=atomic \
  -coverpkg=github.com/rah-0/margo/... ./benchmark
```

See [BENCHMARKS.md](../BENCHMARKS.md) for benchmark execution commands,
methodology, and recorded results.
