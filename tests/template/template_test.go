package template_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
)

func TestCreateGoFileEntity(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := renderer.PathCreateTableDirs([]string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := renderer.CreateGoFileEntity("alpha", tableFields, nil); err != nil {
		t.Fatal(err)
	}

	entityPath := filepath.Join(renderer.OutputPath, "MargoTest", "Alpha", "entity.go")
	content, err := os.ReadFile(entityPath)
	if err != nil {
		t.Fatalf("read generated entity: %v", err)
	}
	if !strings.Contains(string(content), "package Alpha") {
		t.Fatalf("generated entity has unexpected package:\n%s", content)
	}
	assertContextAwareStatementPreparation(t, content)
	if count := strings.Count(string(content), "getPreparedStmt(ctx, query)"); count != 4 {
		t.Errorf("expected four context-aware statement preparation calls, got %d", count)
	}
	testGeneratedPackages(t, renderer.OutputPath)
}

func TestGetFileContentEntityDoesNotCreateOutput(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	content, err := renderer.GetFileContentEntity("alpha", tableFields, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, `"example.com/margo-template-test/output/errs"`) {
		t.Fatal("generated entity does not import shared errors from the consumer module")
	}
	if _, err := os.Stat(renderer.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("content rendering created output: %v", err)
	}
}

func TestCreateGoFilesWithoutModuleDoesNotCreateOutput(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)
	if err := os.Remove(filepath.Join(filepath.Dir(renderer.OutputPath), "go.mod")); err != nil {
		t.Fatal(err)
	}
	if err := renderer.CreateGoFileEntity("alpha", tableFields, nil); !errors.Is(err, errs.ErrGoModuleNotFound) {
		t.Fatalf("entity generation without module: %v", err)
	}
	if _, err := renderer.CreateGoFileQueries(nil, nil); !errors.Is(err, errs.ErrGoModuleNotFound) {
		t.Fatalf("query generation without module: %v", err)
	}
	if _, err := os.Stat(renderer.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("generation without module created output: %v", err)
	}
}
