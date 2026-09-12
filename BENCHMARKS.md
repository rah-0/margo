# Benchmarks

MarGO includes an opt-in benchmark suite comparing generated entity operations
with raw `database/sql`, Bun, Ent, and GORM against the same disposable MariaDB
database.

## Running

Docker must be available. The first run may also download the pinned comparison
dependencies used by the temporary benchmark module. Run these commands from
the repository root. `go -C tests` selects the nested tests module, whose local
replace uses the parent MarGO checkout without requiring `go.work`.

```bash
GOWORK=off go -C tests test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 -v ./benchmark
```

Set `MARGO_BENCHTIME` to control the duration of each sample and
`MARGO_BENCH_COUNT` to collect repeated samples. To reproduce the published
sampling configuration:

```bash
GOWORK=off GOTOOLCHAIN=go1.27.1 MARGO_BENCHTIME=3s MARGO_BENCH_COUNT=5 \
  go -C tests test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 \
    -timeout=15m -v ./benchmark
```

Repeated database samples take several minutes, so this command raises the
outer test timeout. The outer test also owns cancellation and cleanup for the
nested benchmark process.

For a quick compilation and single-iteration smoke run:

```bash
GOWORK=off MARGO_BENCHTIME=1x go -C tests test -tags=benchmark -run '^TestGeneratedBenchmarks$' -count=1 -v ./benchmark
```

The harness starts MariaDB, applies the integration schema, generates the
current MarGO packages, generates the Ent comparator from its schema, runs the
benchmarks with allocation reporting, and removes the temporary module and
container.

## MarGO v0.4.0 results

Lower is better. Each value is the median of five samples with a minimum
benchtime of three seconds per sample. `vs raw` divides the implementation's
median time by the raw SQL median for the same operation; small differences
should not be treated as statistically significant.

**Run:** 2026-09-12 · Go 1.27.1 · linux/amd64 · AMD Ryzen 9 5900HX (16 logical
CPUs) · MariaDB 12.3.3 · Docker 29.7.1

**Comparators:** MySQL driver 1.9.3 · Bun 1.2.14 · Ent 0.14.4 · GORM 1.30.0

### Insert

| Implementation | Time/op | vs raw | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Raw SQL | 2.494 ms | 1.00× | 648 | 18 |
| **MarGO** | 2.541 ms | 1.02× | 1,528 | 25 |
| Bun | 2.431 ms | 0.97× | 5,024 | 14 |
| Ent | 2.538 ms | 1.02× | 3,312 | 73 |
| GORM | 2.522 ms | 1.01× | 5,602 | 58 |

### Delete

| Implementation | Time/op | vs raw | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Raw SQL | 2.358 ms | 1.00× | 216 | 10 |
| **MarGO** | 2.318 ms | 0.98× | 392 | 13 |
| Bun | 2.334 ms | 0.99× | 4,992 | 15 |
| Ent | 2.368 ms | 1.00× | 1,865 | 43 |
| GORM | 2.413 ms | 1.02× | 4,120 | 45 |

### Select

| Implementation | Time/op | vs raw | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| Raw SQL | 114.0 µs | 1.00× | 1,653 | 44 |
| **MarGO** | 117.6 µs | 1.03× | 2,937 | 64 |
| Bun | 129.3 µs | 1.13× | 6,459 | 45 |
| GORM | 241.0 µs | 2.11× | 6,040 | 95 |

These results are a host-specific snapshot, not a universal ranking.

## Cases

| Operation | Implementations |
| --- | --- |
| Insert | Raw SQL, MarGO, Bun, Ent, GORM |
| Delete | Raw SQL, MarGO, Bun, Ent, GORM |
| Select | Raw SQL, MarGO, Bun, GORM |

## Methodology

- Every implementation uses the same MariaDB container and `alpha` table.
- Schema inspection, code generation, and database initialization happen before
  timed operations.
- Timed benchmarks run without race or coverage instrumentation.
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
