package template

import (
	"os"
	"testing"
)

func TestPathCreateOutputDir(t *testing.T) {
	outputPath := setupTemplateTest(t)

	if err := PathCreateOutputDir(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(outputPath); err != nil {
		t.Fatalf("stat output directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", outputPath)
	}
}
