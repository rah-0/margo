//go:build integration

package integration

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const generatedTestModule = "github.com/rah-0/margo/tests/integration/generatedtest"

// The dedicated build tag keeps ordinary builds from compiling this fixture
// before it is copied alongside the generated packages.
//
//go:embed testdata/generated_runtime_test.go
var generatedRuntimeTests []byte

func TestGeneratedCodeRuntime(t *testing.T) {
	database := StartMariaDB(t)
	workspace := t.TempDir()

	writeTestFile(t, filepath.Join(workspace, "go.mod"), []byte(fmt.Sprintf(`module %s

go 1.27

require github.com/go-sql-driver/mysql v1.10.0
`, generatedTestModule)))

	GenerateInto(
		t,
		database,
		workspace,
		filepath.Join(margoRepositoryDir(t), "tests", "integration", "testdata", "queries"),
	)

	writeTestFile(
		t,
		filepath.Join(workspace, "MargoTest", "runtime_test.go"),
		generatedRuntimeTests,
	)

	runTestCommand(
		t,
		workspace,
		append(os.Environ(), "GOWORK=off", "MARGO_INTEGRATION_DSN="+database.DSN),
		"go", "test", "-mod=mod", "-tags=margo_generated_runtime", "-count=1", "-race", "-cover", "-covermode=atomic", "-p=1", "./...",
	)
}

func writeTestFile(t testing.TB, path string, content []byte) {
	t.Helper()

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file %s: %v", filepath.Base(path), err)
	}
}
