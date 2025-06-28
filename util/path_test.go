package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDir(t *testing.T) {
	t.Run("create new nested directory", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "nested", "structure", "final")

		if err := EnsureDir(target); err != nil {
			t.Fatalf("failed to create nested directory: %v", err)
		}

		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("failed to stat created path: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("expected a directory at %q, got something else", target)
		}
	})

	t.Run("ensure existing directory", func(t *testing.T) {
		existing := t.TempDir()

		if err := EnsureDir(existing); err != nil {
			t.Fatalf("EnsureDir failed on existing dir: %v", err)
		}
	})
}
