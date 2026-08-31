package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathCreateDBDir(t *testing.T) {
	outputPath := setupTemplateTest(t)

	if err := PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(outputPath, "MargoTest")
	if info, err := os.Stat(databasePath); err != nil {
		t.Fatalf("stat database directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", databasePath)
	}
}
