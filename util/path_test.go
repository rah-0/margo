package util

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteGoFile(t *testing.T) {
	t.Run("RandomFileWrite", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(t.TempDir(), "*.go")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer tmpFile.Close()

		input := "package main\n\nfunc main(){println(\"hello\")}"
		expected, _ := format.Source([]byte(input))

		err = WriteGoFile(tmpFile.Name(), input)
		if err != nil {
			t.Fatalf("WriteGoFile failed: %v", err)
		}

		data, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if strings.TrimSpace(string(data)) != strings.TrimSpace(string(expected)) {
			t.Errorf("Mismatch\nExpected:\n%s\nGot:\n%s", expected, data)
		}
	})

	t.Run("OverwriteExistingFile", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "samefile.go")

		first := "package main\n\nfunc main(){println(\"first\")}"
		second := "package main\n\nfunc main(){println(\"second\")}"

		err := WriteGoFile(path, first)
		if err != nil {
			t.Fatalf("First write failed: %v", err)
		}

		err = WriteGoFile(path, second)
		if err != nil {
			t.Fatalf("Second write failed: %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		expected, _ := format.Source([]byte(second))
		if strings.TrimSpace(string(data)) != strings.TrimSpace(string(expected)) {
			t.Errorf("File not overwritten correctly\nExpected:\n%s\nGot:\n%s", expected, data)
		}
	})
}
