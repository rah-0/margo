package util

import (
	"go/format"
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
