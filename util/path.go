package util

import (
	"errors"
	"go/format"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/rah-0/nabu"
)

func EnsureDir(path string) error {
	// MkdirAll does nothing if the path already exists as a dir
	if err := os.MkdirAll(path, 0755); err != nil {
		return nabu.FromError(err).WithArgs(path).Log()
	}
	return nil
}

func WriteGoFile(path string, content string) error {
	formatted, err := format.Source([]byte(content))
	if err != nil {
		return nabu.FromError(err).WithArgs(path).Log()
	}
	err = os.WriteFile(path, formatted, 0644)
	if err != nil {
		return nabu.FromError(err).Log()
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
				return "", nabu.FromError(err).WithArgs(curr).Log()
			}
			var modulePath string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "module ") {
					modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
					break
				}
			}
			if modulePath == "" {
				return "", nabu.FromError(errors.New("go.mod found but no module line")).Log()
			}
			relPath, err := filepath.Rel(curr, outputPath)
			if err != nil {
				return "", nabu.FromError(err).WithArgs(outputPath).Log()
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

	return "", nabu.FromError(errors.New("go.mod not found in any parent")).WithArgs(outputPath).Log()
}

// GetSQLFilesInDir returns all .sql file paths in the given directory.
// It only returns direct descendants (non-recursive).
func GetSQLFilesInDir(dirPath string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, nabu.FromError(err).WithArgs(dirPath).Log()
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
