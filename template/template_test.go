package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateGoFileEntity(t *testing.T) {
	outputPath := setupTemplateTest(t)
	if err := PathCreateTableDirs([]string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := CreateGoFileEntity("alpha", tableFields, nil); err != nil {
		t.Fatal(err)
	}

	entityPath := filepath.Join(outputPath, "MargoTest", "Alpha", "entity.go")
	content, err := os.ReadFile(entityPath)
	if err != nil {
		t.Fatalf("read generated entity: %v", err)
	}
	if !strings.Contains(string(content), "package Alpha") {
		t.Fatalf("generated entity has unexpected package:\n%s", content)
	}
}
