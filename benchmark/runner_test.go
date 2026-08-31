//go:build benchmark

// Package benchmark runs MarGO's generated-code benchmarks in an isolated
// temporary module backed by a disposable MariaDB instance.
package benchmark

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/integration"
)

const benchmarkModule = "github.com/rah-0/margo/benchmark/generatedtest"

//go:embed testdata/generated_benchmark_test.go
var generatedBenchmarkTests []byte

//go:embed testdata/ent/schema/alpha.go
var entAlphaSchema []byte

func TestGeneratedBenchmarks(t *testing.T) {
	database := integration.StartMariaDB(t)
	workspace := t.TempDir()

	writeBenchmarkFile(t, filepath.Join(workspace, "go.mod"), []byte(fmt.Sprintf(`module %s

go 1.27

require (
	entgo.io/ent v0.14.4
	github.com/go-sql-driver/mysql v1.9.3
	github.com/google/uuid v1.6.0
	github.com/uptrace/bun v1.2.14
	github.com/uptrace/bun/dialect/mysqldialect v1.2.14
	golang.org/x/tools v0.49.0 // Go 1.27-compatible Ent schema loading
	gorm.io/driver/mysql v1.6.0
	gorm.io/gorm v1.30.0
)
`, benchmarkModule)))

	integration.GenerateInto(t, database, workspace, "")

	benchmarkPath := filepath.Join(workspace, "MargoTest", "benchmark_test.go")
	writeBenchmarkFile(t, benchmarkPath, generatedBenchmarkTests)
	writeBenchmarkFile(t, filepath.Join(workspace, "ent", "schema", "alpha.go"), entAlphaSchema)

	env := benchmarkEnvironment(database.DSN, "")
	runBenchmarkCommand(
		t,
		workspace,
		benchmarkEnvironment(database.DSN, "-tags=margo_generated_benchmark"),
		"go", "run", "-mod=mod", "entgo.io/ent/cmd/ent", "generate", "./ent/schema",
	)

	benchmarkCount := "1"
	if count := os.Getenv("MARGO_BENCH_COUNT"); count != "" {
		benchmarkCount = count
	}
	benchmarkArgs := []string{
		"test", "-mod=mod", "-run", "^$", "-bench", ".", "-benchmem", "-count=" + benchmarkCount,
		"-timeout=0", "-tags=margo_generated_benchmark",
	}
	if benchtime := os.Getenv("MARGO_BENCHTIME"); benchtime != "" {
		benchmarkArgs = append(benchmarkArgs, "-benchtime="+benchtime)
	}
	output := runBenchmarkCommand(
		t,
		filepath.Dir(benchmarkPath),
		env,
		"go", benchmarkArgs...,
	)
	t.Logf("benchmark results:\n%s", output)
}

func benchmarkEnvironment(dsn, goFlags string) []string {
	overrides := map[string]string{
		"GOFLAGS":             goFlags,
		"GOWORK":              "off",
		"MARGO_BENCHMARK_DSN": dsn,
	}

	env := os.Environ()
	result := make([]string, 0, len(env)+len(overrides))
	for _, entry := range env {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func writeBenchmarkFile(t testing.TB, path string, content []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create benchmark fixture directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write benchmark fixture %s: %v", filepath.Base(path), err)
	}
}

func runBenchmarkCommand(t testing.TB, dir string, env []string, name string, args ...string) []byte {
	t.Helper()

	command := exec.CommandContext(t.Context(), name, args...)
	command.Dir = dir
	command.Env = env

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", name, err, output)
	}
	return output
}
