package template_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathCreateDBDir(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)

	if err := renderer.PathCreateDBDir(); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(renderer.OutputPath, "MargoTest")
	if info, err := os.Stat(databasePath); err != nil {
		t.Fatalf("stat database directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", databasePath)
	}
}
