package util

import (
	"fmt"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func EnsureDir(path string) error {
	// MkdirAll does nothing if the path already exists as a dir
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}

func WriteGoFile(path string, content string) error {
	formatted, err := format.Source([]byte(content))
	if err != nil {
		return fmt.Errorf("format Go file %q: %w", path, err)
	}
	err = os.WriteFile(path, formatted, 0644)
	if err != nil {
		return fmt.Errorf("write Go file %q: %w", path, err)
	}
	return nil
}

func ReadFileAsString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func GetGoModuleImportPath(outputPath string) (string, error) {
	curr := filepath.Clean(outputPath)

	for {
		goModPath := filepath.Join(curr, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Read go.mod and extract module path
			data, err := os.ReadFile(goModPath)
			if err != nil {
				return "", fmt.Errorf("read %q: %w", goModPath, err)
			}
			var modulePath string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "module ") {
					modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
					break
				}
			}
			if modulePath == "" {
				return "", fmt.Errorf("%s: no module directive", goModPath)
			}
			relPath, err := filepath.Rel(curr, outputPath)
			if err != nil {
				return "", fmt.Errorf("resolve output path %q relative to %q: %w", outputPath, curr, err)
			}
			importPath := path.Join(modulePath, filepath.ToSlash(relPath))
			return importPath, nil
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break // reached filesystem root
		}
		curr = parent
	}

	return "", fmt.Errorf("go.mod not found in any parent of %q", outputPath)
}

// GetSQLFilesInDir returns all .sql file paths in the given directory.
// It only returns direct descendants (non-recursive).
func GetSQLFilesInDir(dirPath string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("read SQL directory %q: %w", dirPath, err)
	}

	var sqlFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			sqlFiles = append(sqlFiles, filepath.Join(dirPath, entry.Name()))
		}
	}

	return sqlFiles, nil
}
