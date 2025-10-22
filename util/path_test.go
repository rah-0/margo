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

	importPath, err := GetGoModuleImportPath(nestedPath)
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

func TestGetSQLFilesInDir(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(files) != 0 {
			t.Errorf("expected 0 files, got %d", len(files))
		}
	})

	t.Run("directory with only .sql files", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test .sql files
		sqlFiles := []string{"query1.sql", "query2.sql", "migration.sql"}
		for _, name := range sqlFiles {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("SELECT 1;"), 0644); err != nil {
				t.Fatalf("failed to create test file %s: %v", name, err)
			}
		}

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(files) != len(sqlFiles) {
			t.Errorf("expected %d files, got %d", len(sqlFiles), len(files))
		}

		// Verify all expected files are present
		fileMap := make(map[string]bool)
		for _, f := range files {
			fileMap[filepath.Base(f)] = true
		}

		for _, expected := range sqlFiles {
			if !fileMap[expected] {
				t.Errorf("expected file %s not found in results", expected)
			}
		}
	})

	t.Run("directory with mixed file types", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create various file types
		testFiles := map[string]bool{
			"query1.sql":    true,  // should be included
			"query2.SQL":    true,  // should be included (uppercase)
			"readme.md":     false, // should be excluded
			"script.sh":     false, // should be excluded
			"data.json":     false, // should be excluded
			"migration.sql": true,  // should be included
			"test.go":       false, // should be excluded
			"notes.txt":     false, // should be excluded
		}

		for name := range testFiles {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
				t.Fatalf("failed to create test file %s: %v", name, err)
			}
		}

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Count expected SQL files
		expectedCount := 0
		for _, isSql := range testFiles {
			if isSql {
				expectedCount++
			}
		}

		if len(files) != expectedCount {
			t.Errorf("expected %d SQL files, got %d", expectedCount, len(files))
		}

		// Verify only .sql files are returned
		for _, f := range files {
			base := filepath.Base(f)
			shouldInclude, exists := testFiles[base]
			if !exists {
				t.Errorf("unexpected file in results: %s", base)
			}
			if !shouldInclude {
				t.Errorf("non-SQL file included in results: %s", base)
			}
		}
	})

	t.Run("directory with subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files in root
		rootFile := filepath.Join(tmpDir, "root.sql")
		if err := os.WriteFile(rootFile, []byte("SELECT 1;"), 0644); err != nil {
			t.Fatalf("failed to create root file: %v", err)
		}

		// Create subdirectory with SQL file
		subDir := filepath.Join(tmpDir, "subdir")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("failed to create subdirectory: %v", err)
		}

		subFile := filepath.Join(subDir, "nested.sql")
		if err := os.WriteFile(subFile, []byte("SELECT 2;"), 0644); err != nil {
			t.Fatalf("failed to create nested file: %v", err)
		}

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should only return files in the root directory, not subdirectories
		if len(files) != 1 {
			t.Errorf("expected 1 file (non-recursive), got %d", len(files))
		}

		if len(files) > 0 && filepath.Base(files[0]) != "root.sql" {
			t.Errorf("expected root.sql, got %s", filepath.Base(files[0]))
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		nonExistent := filepath.Join(os.TempDir(), "non-existent-dir-12345")

		_, err := GetSQLFilesInDir(nonExistent)
		if err == nil {
			t.Error("expected error for non-existent directory, got nil")
		}
	})

	t.Run("case insensitive extension matching", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files with different case extensions
		testFiles := []string{
			"lower.sql",
			"upper.SQL",
			"mixed.Sql",
			"mixed2.sQl",
		}

		for _, name := range testFiles {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte("SELECT 1;"), 0644); err != nil {
				t.Fatalf("failed to create test file %s: %v", name, err)
			}
		}

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(files) != len(testFiles) {
			t.Errorf("expected %d files (case insensitive), got %d", len(testFiles), len(files))
		}
	})

	t.Run("returns full paths", func(t *testing.T) {
		tmpDir := t.TempDir()

		fileName := "test.sql"
		filePath := filepath.Join(tmpDir, fileName)
		if err := os.WriteFile(filePath, []byte("SELECT 1;"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		files, err := GetSQLFilesInDir(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}

		// Verify it's a full path, not just the filename
		if !filepath.IsAbs(files[0]) {
			t.Errorf("expected absolute path, got relative: %s", files[0])
		}

		if files[0] != filePath {
			t.Errorf("expected path %s, got %s", filePath, files[0])
		}
	})
}
