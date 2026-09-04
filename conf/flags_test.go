package conf

import (
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func resetFlags(t *testing.T) {
	t.Helper()
	origArgs := os.Args
	origCmd := flag.CommandLine
	t.Cleanup(func() {
		os.Args = origArgs
		flag.CommandLine = origCmd
	})
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
}

func validArgs(extra ...string) []string {
	base := []string{
		"margo",
		"-dbUser", "u",
		"-dbPassword", "p",
		"-dbName", "n",
		"-dbIp", "127.0.0.1",
		"-dbPort", "3306",
		"-outputPath", "/tmp",
	}
	return append(base, extra...)
}

func TestCheckFlagsMissingArgs(t *testing.T) {
	resetFlags(t)
	os.Args = []string{"margo"}
	if err := CheckFlags(); !errors.Is(err, ErrMissingArgs) {
		t.Fatalf("expected ErrMissingArgs, got %v", err)
	}
}

func TestCheckFlagsQueriesPathInvalid(t *testing.T) {
	resetFlags(t)
	os.Args = validArgs("-queriesPath", filepath.Join(t.TempDir(), "does-not-exist"))
	err := CheckFlags()
	if !errors.Is(err, ErrQueriesPathInvalid) {
		t.Fatalf("expected ErrQueriesPathInvalid, got %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestCheckFlagsQueriesPathNotDir(t *testing.T) {
	resetFlags(t)
	file := filepath.Join(t.TempDir(), "queries.sql")
	if err := os.WriteFile(file, []byte("-- noop"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Args = validArgs("-queriesPath", file)
	if err := CheckFlags(); !errors.Is(err, ErrQueriesPathNotDir) {
		t.Fatalf("expected ErrQueriesPathNotDir, got %v", err)
	}
}

func TestCheckFlagsValid(t *testing.T) {
	resetFlags(t)
	os.Args = validArgs("-queriesPath", t.TempDir())
	if err := CheckFlags(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
