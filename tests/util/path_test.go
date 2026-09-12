package util_test

import (
	"errors"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/util"
)

func TestEnsureDir(t *testing.T) {
	t.Run("create new nested directory", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "nested", "structure", "final")

		if err := util.EnsureDir(target); err != nil {
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

		if err := util.EnsureDir(existing); err != nil {
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

		err = util.WriteGoFile(tmpFile.Name(), input)
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

		err := util.WriteGoFile(path, first)
		if err != nil {
			t.Fatalf("First write failed: %v", err)
		}

		err = util.WriteGoFile(path, second)
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

func TestGetGoModuleImportPath(t *testing.T) {
	tmpRoot, err := os.MkdirTemp("", "gomodtest")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpRoot)

	moduleName := "github.com/example/project"

	// Create go.mod file at root
	goModPath := filepath.Join(tmpRoot, "go.mod")
	goModContent := "module " + moduleName + "\n"
	if err := os.WriteFile(goModPath, []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Create nested directories: /a/b/c
	nestedPath := filepath.Join(tmpRoot, "a", "b", "c")
	if err := os.MkdirAll(nestedPath, 0755); err != nil {
		t.Fatalf("failed to create nested dirs: %v", err)
	}

	importPath, err := util.GetGoModuleImportPath(nestedPath)
	if err != nil {
		t.Fatalf("unexpected error from GetGoModuleImportPath: %v", err)
	}

	expectedSuffix := "a/b/c"
	if !strings.HasSuffix(importPath, expectedSuffix) {
		t.Errorf("expected import path to end with %q, got %q", expectedSuffix, importPath)
	}

	expectedFull := moduleName + "/a/b/c"
	if importPath != expectedFull {
		t.Errorf("expected full import path %q, got %q", expectedFull, importPath)
	}
}

func TestGetGoModuleImportPathErrors(t *testing.T) {
	t.Run("module file missing", func(t *testing.T) {
		output := t.TempDir()
		_, err := util.GetGoModuleImportPath(output)
		if !errors.Is(err, errs.ErrGoModuleNotFound) || !strings.Contains(err.Error(), output) {
			t.Fatalf("module discovery error = %v, want ErrGoModuleNotFound with output path", err)
		}
	})

	for _, content := range []string{"go 1.27\n", "module \n"} {
		t.Run("module directive missing", func(t *testing.T) {
			output := t.TempDir()
			module := filepath.Join(output, "go.mod")
			if err := os.WriteFile(module, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := util.GetGoModuleImportPath(output)
			if !errors.Is(err, errs.ErrModuleDirectiveNotFound) || !strings.Contains(err.Error(), module) {
				t.Fatalf("module discovery error = %v, want ErrModuleDirectiveNotFound with file path", err)
			}
		})
	}
}
