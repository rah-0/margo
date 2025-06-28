package util

import (
	"os"

	"github.com/rah-0/nabu"
)

func EnsureDir(path string) error {
	// MkdirAll does nothing if the path already exists as a dir
	if err := os.MkdirAll(path, 0755); err != nil {
		return nabu.FromError(err).WithArgs(path).Log()
	}
	return nil
}
