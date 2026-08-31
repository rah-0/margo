# Benchmarks

MarGO includes an opt-in benchmark suite comparing generated entity operations
with raw `database/sql`, Bun, Ent, and GORM against the same disposable MariaDB
database.

## Running

Docker must be available. The first run may also download the pinned comparison
dependencies used by the temporary benchmark module.

```bash
go test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 -v ./benchmark
```

Set `MARGO_BENCHTIME` to control the duration of each sample and
`MARGO_BENCH_COUNT` to collect repeated samples. To reproduce the published
sampling configuration:

```bash
MARGO_BENCHTIME=3s MARGO_BENCH_COUNT=5 \
  go test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 \
    -timeout=15m -v ./benchmark
```

Repeated database samples take several minutes, so this command raises the
outer test timeout. The outer test also owns cancellation and cleanup for the
nested benchmark process.

For a quick compilation and single-iteration smoke run:

```bash
MARGO_BENCHTIME=1x go test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 -v ./benchmark
```

The harness starts MariaDB, applies the integration schema, generates the
current MarGO packages, generates the Ent comparator from its schema, runs the
benchmarks with allocation reporting, and removes the temporary module and
container.

## Latest Results

Lower is better. Each value is the median of five samples with a minimum
benchtime of three seconds per sample. `vs raw` divides the implementation's
median time by the raw SQL median for the same operation; small differences
should not be treated as statistically significant.

**Run:** 2026-08-31 · Go 1.27.0 · linux/amd64 · AMD Ryzen 9 5900HX (16 logical
CPUs) · MariaDB 12.3.3

**Comparators:** MySQL driver 1.9.3 · Bun 1.2.14 · Ent 0.14.4 · GORM 1.30.0

| Operation | Implementation | Time/op | vs raw | B/op | allocs/op |
| --- | --- | ---: | ---: | ---: | ---: |
| Insert | Raw SQL | 2.358 ms | 1.00× | 648 | 18 |
| Insert | **MarGO** | 2.442 ms | 1.04× | 1,528 | 25 |
| Insert | Bun | 2.549 ms | 1.08× | 5,024 | 14 |
| Insert | Ent | 2.501 ms | 1.06× | 3,313 | 73 |
| Insert | GORM | 2.517 ms | 1.07× | 5,604 | 58 |
| Delete | Raw SQL | 2.338 ms | 1.00× | 216 | 10 |
| Delete | **MarGO** | 2.305 ms | 0.99× | 392 | 13 |
| Delete | Bun | 2.323 ms | 0.99× | 4,992 | 15 |
| Delete | Ent | 2.409 ms | 1.03× | 1,864 | 43 |
| Delete | GORM | 2.396 ms | 1.02× | 4,122 | 45 |
| Select | Raw SQL | 114.5 µs | 1.00× | 1,654 | 44 |
| Select | **MarGO** | 116.6 µs | 1.02× | 2,934 | 64 |
| Select | Bun | 130.6 µs | 1.14× | 6,455 | 45 |
| Select | GORM | 243.6 µs | 2.13× | 6,036 | 95 |

These values come from the suite and methodology in this repository revision.
They are a host-specific snapshot, not a universal ranking.

## Cases

| Operation | Implementations |
| --- | --- |
| Insert | Raw SQL, MarGO, Bun, Ent, GORM |
| Delete | Raw SQL, MarGO, Bun, Ent, GORM |
| Select | Raw SQL, MarGO, Bun, GORM |

## Methodology

- Every implementation uses the same MariaDB container and `alpha` table.
- Insert timings include UUID and model construction for every implementation.
- Delete and select fixtures are seeded outside the measured interval.
- Select benchmarks materialize the complete row rather than checking only for
  existence.
- Raw SQL statements are prepared before timing. MarGO uses its generated
  prepared-statement cache.
- Benchmarks run serially to avoid cross-implementation database contention.
- Bun, Ent, GORM, and their dependencies are confined to the temporary module;
  they are not runtime dependencies of MarGO.

Database benchmark values vary with the host, container runtime, MariaDB
version, and current system load. Compare implementations from the same run
rather than comparing numbers produced on different machines.
