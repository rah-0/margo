package conf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// validateOutputPath checks existing ancestors without creating directories.
// Preserve the supplied path: cleaning "file/../output" would hide an invalid
// component, and cleaning "link/../output" could change its destination.
func validateOutputPath(outputPath string) error {
	path := outputPath
	for {
		info, err := os.Stat(path)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%w: %q", ErrOutputPathNotDir, path)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %q: %w", ErrOutputPathInvalid, outputPath, err)
		}
		if _, linkErr := os.Lstat(path); linkErr == nil {
			return fmt.Errorf("%w: dangling symbolic link %q", ErrOutputPathInvalid, path)
		} else if !errors.Is(linkErr, os.ErrNotExist) {
			return fmt.Errorf("%w: %q: %w", ErrOutputPathInvalid, outputPath, linkErr)
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
			return fmt.Errorf("%w: %q: %w", ErrOutputPathInvalid, outputPath, err)
		}
		path = parent
	}
}
