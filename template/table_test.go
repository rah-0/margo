package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathCreateTableDirs(t *testing.T) {
	outputPath := setupTemplateTest(t)

	if err := PathCreateTableDirs(tableNames); err != nil {
		t.Fatal(err)
	}
	for _, tableName := range []string{"Alpha", "Beta"} {
		tablePath := filepath.Join(outputPath, "MargoTest", tableName)
		if info, err := os.Stat(tablePath); err != nil {
			t.Fatalf("stat table directory %s: %v", tableName, err)
		} else if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", tablePath)
		}
	}
}
