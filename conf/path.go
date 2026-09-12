package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rah-0/margo/errs"
)

// ValidatePaths validates operation paths without creating directories or
// changing the supplied paths. Empty paths disable their respective operations.
func ValidatePaths(outputPath, queriesPath, migrationsPath string) error {
	if queriesPath != "" && outputPath == "" {
		return errs.ErrQueriesWithoutOutput
	}
	if outputPath != "" {
		if err := validateOutputPath(outputPath); err != nil {
			return err
		}
	}
	for _, input := range []struct {
		path    string
		invalid error
		notDir  error
	}{
		{queriesPath, errs.ErrQueriesPathInvalid, errs.ErrQueriesPathNotDir},
		{migrationsPath, errs.ErrMigrationsPathInvalid, errs.ErrMigrationsPathNotDir},
	} {
		if input.path == "" {
			continue
		}
		info, err := os.Stat(input.path)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", input.invalid, input.path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %q", input.notDir, input.path)
		}
	}
	return nil
}

// validateOutputPath checks existing ancestors without creating directories.
// Preserve the supplied path: cleaning "file/../output" would hide an invalid
// component, and cleaning "link/../output" could change its destination.
func validateOutputPath(outputPath string) error {
	path := outputPath
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%w: %q", errs.ErrOutputPathNotDir, path)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %q: %w", errs.ErrOutputPathInvalid, outputPath, err)
		}
		if _, linkErr := os.Lstat(path); linkErr == nil {
			return fmt.Errorf("%w: dangling symbolic link %q", errs.ErrOutputPathInvalid, path)
		} else if !errors.Is(linkErr, os.ErrNotExist) {
			return fmt.Errorf("%w: %q: %w", errs.ErrOutputPathInvalid, outputPath, linkErr)
		}

		// Split preserves dots and symlinks; filepath.Dir would clean them away.
		parent, _ := filepath.Split(path)
		rootLength := len(filepath.VolumeName(parent)) + 1
		for len(parent) > rootLength && os.IsPathSeparator(parent[len(parent)-1]) {
			parent = parent[:len(parent)-1]
		}
		if parent == "" {
			parent = "."
		}
		if parent == path {
			return fmt.Errorf("%w: %q: %w", errs.ErrOutputPathInvalid, outputPath, err)
		}
		path = parent
	}
}
