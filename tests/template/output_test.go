package template_test

import (
	"os"
	"testing"
)

func TestPathCreateOutputDir(t *testing.T) {
	t.Parallel()
	renderer := setupTemplateTest(t)

	if err := renderer.PathCreateOutputDir(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(renderer.OutputPath); err != nil {
		t.Fatalf("stat output directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", renderer.OutputPath)
	}
}
